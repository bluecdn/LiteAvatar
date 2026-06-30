// LiteAvatar —— 多源头像聚合代理
//
// 并发探测 Cravatar / Cnavatar / WeAvatar / Gravatar 邮箱头像源，纯数字 ID 走腾讯 QQ 头像，
// 第一个命中的立即采用；全部未命中回退内置默认头像。命中头像转 AVIF 落盘缓存，
// 按请求 Accept 头协商返回 AVIF 或原图(webp/jpeg/png)。
//
// 接口:  GET /avatar/{id}?s={size}&d={default}
//   - id 为 32-64 位十六进制 → 邮箱头像 (md5 / sha256)，并发探测各源
//   - id 为 5-12 位纯数字   → 腾讯 QQ 头像
//   - GET /stats.php        → 累计请求数 (JSON)
//   - GET /healthz          → 健康检查
//
// gravatar 上游可配置(-gravatar-upstream)：硅谷节点直连 secure.gravatar.com，
// 上海等被墙节点改走硅谷的 gravatar-us.bluecdn.com 中转。
package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	_ "embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gen2brain/avif"
	_ "golang.org/x/image/webp"
)

const (
	defaultListen = "127.0.0.1:8787"
	userAgent     = "LiteAvatar/1.0 (+https://gravatar.bluecdn.com)"
	cacheMaxAge   = 15 * 24 * 60 * 60 // 15 天，与边缘 CDN 缓存一致
	maxSize       = 2048
	defaultSize   = 80
	probeTimeout  = 5 * time.Second
	fetchTimeout  = 10 * time.Second
	persistEvery  = 30 * time.Second
	cacheTTL      = 30 * 24 * time.Hour // 落盘缓存保留期
)

type source struct{ name, base string }

// 邮箱头像源：并发探测，第一个命中的立即采用。
// 国内镜像在前、gravatar 兜底——国内节点不会被 gravatar 被墙的超时拖慢。
// gravatar 的 base 由 -gravatar-upstream 决定（在 main 中注入）。
var emailSources []source

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
	cacheDir     string
	avifQuality  int
)

var errNoSource = errors.New("no source hit")

func main() {
	listen := flag.String("listen", defaultListen, "监听地址")
	cf := flag.String("counter", "static/stats/requests.count", "请求计数持久化文件")
	upstream := flag.String("gravatar-upstream", "https://secure.gravatar.com", "gravatar 源(被墙节点设为硅谷中转 https://gravatar-us.bluecdn.com)")
	cd := flag.String("cache-dir", "cache", "AVIF 缓存目录")
	q := flag.Int("avif-quality", 55, "AVIF 质量 0-100")
	flag.Parse()
	counterFile = *cf
	cacheDir = *cd
	avifQuality = *q

	emailSources = []source{
		{"cravatar", "https://cravatar.com"},
		{"cnavatar", "https://cnavatar.com"},
		{"weavatar", "https://weavatar.com"},
		{"gravatar", strings.TrimRight(*upstream, "/")},
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		log.Printf("warn: 无法创建缓存目录 %s: %v", cacheDir, err)
	}
	loadCounter()
	go persistCounterLoop()
	go cleanupLoop()

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
	log.Printf("LiteAvatar listening on %s | gravatar-upstream=%s | cache=%s", *listen, emailSources[3].base, cacheDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// avatarHandler 解析 /avatar/{id} 并按 id 形态分流，统一走缓存+AVIF 协商。
func avatarHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&requestCount, 1)

	id := strings.TrimPrefix(r.URL.Path, "/avatar/")
	id = strings.TrimSuffix(id, ".jpg")
	id = strings.TrimSuffix(id, ".png")
	id = strings.TrimSuffix(id, ".avif")
	id = strings.ToLower(strings.TrimSpace(id))

	size := parseSize(r.URL.Query().Get("s"))
	def := r.URL.Query().Get("d")

	switch {
	case qqRe.MatchString(id):
		serveAvatar(w, r, fmt.Sprintf("qq:%s:%d", id, size), func(ctx context.Context) ([]byte, string, string, error) {
			return fetchQQ(ctx, id, size)
		})
	case emailHashRe.MatchString(id):
		serveAvatar(w, r, fmt.Sprintf("email:%s:%d", id, size), func(ctx context.Context) ([]byte, string, string, error) {
			return fetchGravatar(ctx, id, size, def)
		})
	default:
		writeDefault(w)
	}
}

// serveAvatar 统一处理：缓存命中直接按 Accept 返回；未命中则回源拉取、转 AVIF 落盘、再返回。
func serveAvatar(w http.ResponseWriter, r *http.Request, key string, fetch func(context.Context) ([]byte, string, string, error)) {
	avifPath, origPath, ctPath := cachePaths(key)

	// 缓存命中
	if avifData, err := os.ReadFile(avifPath); err == nil {
		if acceptsAVIF(r) {
			touch(avifPath)
			output(w, avifData, "image/avif", "cache", "HIT")
			return
		}
		if orig, err := os.ReadFile(origPath); err == nil {
			touch(origPath)
			output(w, orig, readCT(ctPath), "cache", "HIT")
			return
		}
	}

	// 未命中：回源
	body, ct, src, err := fetch(r.Context())
	if err != nil || len(body) == 0 {
		writeDefault(w)
		return
	}

	// 转 AVIF 并落盘
	avifData, aerr := toAVIF(body)
	writeFileAtomic(origPath, body)
	writeFileAtomic(ctPath, []byte(ct))
	if aerr == nil {
		writeFileAtomic(avifPath, avifData)
	}

	if acceptsAVIF(r) && aerr == nil {
		output(w, avifData, "image/avif", src, "MISS")
	} else {
		output(w, body, ct, src, "MISS")
	}
}

// fetchGravatar 并发探测邮箱源，命中后回源拉取真实头像。
func fetchGravatar(ctx context.Context, hash string, size int, def string) ([]byte, string, string, error) {
	src := probeSources(ctx, hash)
	if src.base == "" {
		return nil, "", "", errNoSource
	}
	url := fmt.Sprintf("%s/avatar/%s?s=%d&d=%s", src.base, hash, size, defaultParam(def))
	body, ct, err := httpGet(ctx, url)
	return body, ct, src.name, err
}

// fetchQQ 拉取腾讯 QQ 头像。
func fetchQQ(ctx context.Context, qq string, size int) ([]byte, string, string, error) {
	url := fmt.Sprintf("https://q.qlogo.cn/headimg_dl?dst_uin=%s&spec=%d&img_type=jpg", qq, pickQQSpec(size))
	body, ct, err := httpGet(ctx, url)
	return body, ct, "qq", err
}

// probeSources 并发探测各邮箱源，第一个命中的立即采用（并取消其余探测）。
// 探测请求 {base}/avatar/{hash}?s=1&d=404：HTTP 200 即视为存在真实头像。
func probeSources(ctx context.Context, hash string) source {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	hits := make(chan int, len(emailSources))
	for i, s := range emailSources {
		go func(i int, base string) {
			idx := -1
			if probe(ctx, fmt.Sprintf("%s/avatar/%s?s=1&d=404", base, hash)) {
				idx = i
			}
			select {
			case hits <- idx:
			case <-ctx.Done():
			}
		}(i, s.base)
	}

	for remaining := len(emailSources); remaining > 0; remaining-- {
		select {
		case idx := <-hits:
			if idx >= 0 {
				return emailSources[idx]
			}
		case <-ctx.Done():
			return source{}
		}
	}
	return source{}
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

// toAVIF 解码原图(jpeg/png/gif/webp)并编码为 AVIF。
func toAVIF(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := avif.Encode(&buf, img, avif.Options{Quality: avifQuality, Speed: 8}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func acceptsAVIF(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "image/avif")
}

// output 输出图片，带缓存头、来源标记、缓存命中标记，并声明 Vary: Accept 供 CDN 分缓存。
func output(w http.ResponseWriter, body []byte, contentType, source, cache string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cacheMaxAge))
	w.Header().Set("Vary", "Accept")
	w.Header().Set("X-Avatar-Source", source)
	w.Header().Set("X-Cache", cache)
	w.Write(body)
}

func writeDefault(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(defaultAvatar)))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Vary", "Accept")
	w.Header().Set("X-Avatar-Source", "default")
	w.Write(defaultAvatar)
}

// ---- 缓存辅助 ----

func cachePaths(key string) (avifPath, origPath, ctPath string) {
	sum := sha1.Sum([]byte(key))
	s := hex.EncodeToString(sum[:])
	dir := filepath.Join(cacheDir, s[:2])
	os.MkdirAll(dir, 0o755)
	base := filepath.Join(dir, s)
	return base + ".avif", base + ".orig", base + ".ct"
}

func writeFileAtomic(path string, data []byte) {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err == nil {
		os.Rename(tmp, path)
	}
}

func readCT(path string) string {
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return string(b)
	}
	return "image/jpeg"
}

func touch(path string) {
	now := time.Now()
	os.Chtimes(path, now, now)
}

// cleanupLoop 定期清理超过 cacheTTL 未访问的缓存文件。
func cleanupLoop() {
	t := time.NewTicker(12 * time.Hour)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-cacheTTL)
		filepath.Walk(cacheDir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && info.ModTime().Before(cutoff) {
				os.Remove(p)
			}
			return nil
		})
	}
}

// ---- 参数与统计 ----

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
