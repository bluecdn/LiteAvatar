// LiteAvatar —— 多源头像聚合代理
//
// 并发探测 Cravatar / Gravatar / WeAvatar 三大邮箱头像源，纯数字 ID 走腾讯 QQ 头像，
// 自动返回第一个命中的真实头像；全部未命中时回退本地默认头像。
//
// 接口:  GET /avatar/{id}?s={size}&d={default}
//   - id 为 32-64 位十六进制 → 邮箱头像 (md5 / sha256)，并发探测三源
//   - id 为 5-12 位纯数字   → 腾讯 QQ 头像
//   - GET /stats.php        → 累计请求数 (JSON)
//   - GET /healthz          → 健康检查
//
// 部署于能直连各头像上游的境外节点 (如硅谷)，前置 Caddy + CDN 做边缘缓存。
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultListen = "127.0.0.1:8787"
	userAgent     = "LiteAvatar/1.0 (+https://gravatar.bluecdn.com)"
	cacheMaxAge   = 15 * 24 * 60 * 60 // 15 天，与边缘 CDN 缓存保持一致
	maxSize       = 2048
	defaultSize   = 80
	probeTimeout  = 5 * time.Second
	fetchTimeout  = 10 * time.Second
	persistEvery  = 30 * time.Second
)

// 邮箱头像源，按优先级排列：并发探测，多个命中时取靠前者。
var emailSources = []struct {
	name string
	base string
}{
	{"cravatar", "https://cn.cravatar.com"},
	{"gravatar", "https://secure.gravatar.com"},
	{"weavatar", "https://weavatar.com"},
}

var (
	emailHashRe = regexp.MustCompile(`^[a-f0-9]{32,64}$`)
	qqRe        = regexp.MustCompile(`^\d{5,12}$`)
)

//go:embed static/default-avatar.png
var defaultAvatar []byte

var httpClient = &http.Client{
	Timeout: fetchTimeout,
	CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	},
}

var (
	requestCount int64
	counterFile  string
)

func main() {
	listen := flag.String("listen", defaultListen, "监听地址")
	cf := flag.String("counter", "static/stats/requests.count", "请求计数持久化文件")
	flag.Parse()
	counterFile = *cf

	loadCounter()
	go persistCounterLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/avatar/", avatarHandler)
	mux.HandleFunc("/stats.php", statsHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "ok")
	})

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("avatar proxy listening on %s", *listen)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// avatarHandler 解析 /avatar/{id} 并按 id 形态分流。
func avatarHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&requestCount, 1)

	id := strings.TrimPrefix(r.URL.Path, "/avatar/")
	id = strings.TrimSuffix(id, ".jpg")
	id = strings.TrimSuffix(id, ".png")
	id = strings.ToLower(strings.TrimSpace(id))

	size := parseSize(r.URL.Query().Get("s"))
	def := r.URL.Query().Get("d")

	switch {
	case qqRe.MatchString(id):
		handleQQ(w, r, id, size)
	case emailHashRe.MatchString(id):
		handleGravatar(w, r, id, size, def)
	default:
		writeDefault(w)
	}
}

// handleGravatar 并发探测三源，命中则回源拉取真实头像，否则回退默认头像。
func handleGravatar(w http.ResponseWriter, r *http.Request, hash string, size int, def string) {
	src := probeSources(r.Context(), hash)
	if src.base == "" {
		writeDefault(w)
		return
	}
	url := fmt.Sprintf("%s/avatar/%s?s=%d&d=%s", src.base, hash, size, defaultParam(def))
	body, ct, err := httpGet(r.Context(), url)
	if err != nil {
		writeDefault(w)
		return
	}
	outputImage(w, body, ct, src.name)
}

// handleQQ 拉取腾讯 QQ 头像。
func handleQQ(w http.ResponseWriter, r *http.Request, qq string, size int) {
	url := fmt.Sprintf("https://q.qlogo.cn/headimg_dl?dst_uin=%s&spec=%d&img_type=jpg", qq, pickQQSpec(size))
	body, ct, err := httpGet(r.Context(), url)
	if err != nil || len(body) == 0 {
		writeDefault(w)
		return
	}
	outputImage(w, body, ct, "qq")
}

// probeSources 并发探测各邮箱源，返回优先级最高的命中源。
// 探测请求 {base}/avatar/{hash}?s=1&d=404：HTTP 200 即视为存在真实头像。
func probeSources(ctx context.Context, hash string) struct{ name, base string } {
	type result struct{ idx int }
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	ch := make(chan result, len(emailSources))
	var wg sync.WaitGroup
	for i, s := range emailSources {
		wg.Add(1)
		go func(i int, base string) {
			defer wg.Done()
			url := fmt.Sprintf("%s/avatar/%s?s=1&d=404", base, hash)
			if probe(ctx, url) {
				ch <- result{i}
			}
		}(i, s.base)
	}
	go func() { wg.Wait(); close(ch) }()

	best := -1
	for r := range ch {
		if best == -1 || r.idx < best {
			best = r.idx
		}
	}
	if best == -1 {
		return struct{ name, base string }{}
	}
	return struct{ name, base string }{emailSources[best].name, emailSources[best].base}
}

func probe(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	return resp.StatusCode == http.StatusOK
}

// httpGet 回源拉取图片，返回 body 与 Content-Type。
func httpGet(ctx context.Context, url string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 上限 8MB
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		ct = "image/jpeg"
	}
	return body, ct, nil
}

// outputImage 输出图片并打上缓存与来源标记。
func outputImage(w http.ResponseWriter, body []byte, contentType, source string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cacheMaxAge))
	w.Header().Set("X-Avatar-Source", source)
	w.Write(body)
}

// writeDefault 输出内置默认头像。
func writeDefault(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(defaultAvatar)))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Avatar-Source", "default")
	w.Write(defaultAvatar)
}

// pickQQSpec 按目标尺寸选择 QQ 头像规格 (qlogo headimg_dl 的 spec 参数)。
func pickQQSpec(size int) int {
	switch {
	case size <= 40:
		return 40
	case size <= 100:
		return 100
	case size <= 140:
		return 140
	default:
		return 640
	}
}

// parseSize 解析并夹取尺寸到 [1, maxSize]。
func parseSize(s string) int {
	if s == "" {
		return defaultSize
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultSize
	}
	if n > maxSize {
		return maxSize
	}
	return n
}

// defaultParam 透传给上游的 d 参数，缺省给 404（probe 已确认头像存在，不会触发）。
func defaultParam(d string) string {
	if d == "" {
		return "404"
	}
	return d
}

func statsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, `{"requests":%d}`, atomic.LoadInt64(&requestCount))
}

func loadCounter() {
	b, err := os.ReadFile(counterFile)
	if err != nil {
		return
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil {
		atomic.StoreInt64(&requestCount, n)
	}
}

func persistCounter() {
	n := atomic.LoadInt64(&requestCount)
	tmp := counterFile + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.FormatInt(n, 10)), 0o644); err == nil {
		os.Rename(tmp, counterFile)
	}
}

func persistCounterLoop() {
	t := time.NewTicker(persistEvery)
	defer t.Stop()
	for range t.C {
		persistCounter()
	}
}
