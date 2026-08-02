// LiteAvatar —— 多源头像聚合代理
//
// 邮箱头像按优先级串行回源：gravatar → cravatar → weavatar → cnavatar，
// 哪个先有真实头像(非默认、非404)就用哪个；纯数字 ID(前端检测到的 QQ 邮箱→QQ号)走腾讯 QQ 头像。
// 拿到真实头像后转 AVIF 落盘缓存，按 Accept 头协商返回 AVIF 或原图；缓存有效期可配。
//
// 接口:  GET /avatar/{id}?s={size}
//   - id 为 32-64 位十六进制 → 邮箱头像 (md5 / sha256)，串行四源
//   - id 为 5-12 位纯数字   → 腾讯 QQ 头像
//   - GET /stats(统计页+JSON) / /healthz
//   - GET /                → 首页(从 -site-dir 读取 index.html 与 public 资源)
//
// gravatar 上游可配置(-gravatar-upstream)：硅谷直连 secure.gravatar.com，
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
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/avif"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	defaultListen = "127.0.0.1:8787"
	userAgent     = "LiteAvatar/1.0 (+https://gravatar.bluecdn.com)"
	maxSize       = 2048
	defaultSize   = 80
	fetchTimeout  = 8 * time.Second  // 单个源回源超时
	totalTimeout  = 18 * time.Second // 串行所有源的总超时
	persistEvery  = 30 * time.Second
	cacheVersion  = "v2"
)

type source struct{ name, base string }

// 邮箱头像源，严格按优先级串行：gravatar 第一(最准),依次降级到国内镜像。
// 哪个先返回真实头像(d=404 下非 404)就用它。gravatar 的 base 由 -gravatar-upstream 注入。
var emailSources []source

var (
	emailHashRe = regexp.MustCompile(`^[a-f0-9]{32,64}$`)
	qqRe        = regexp.MustCompile(`^\d{5,12}$`)
)

//go:embed static/default-avatar.avif
var defaultAvatar []byte

var (
	defaultAvatarOnce  sync.Once
	defaultAvatarImage image.Image
	defaultAvatarErr   error
	defaultRenderCache = struct {
		sync.Mutex
		entries map[string][]byte
		order   []string
	}{entries: make(map[string][]byte)}
	cnavatarPlaceholderHashes sync.Map
)

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
	cacheTTL     time.Duration // 缓存有效期：命中且未过期直接返回，过期则重新回源
	esaFile      string        // 阿里云 ESA 累计请求数文件（由后台脚本定时写入）
	siteDir      string        // 首页与 public 静态资源目录
)

var errNoSource = errors.New("no source hit")

const cnavatarProbeHash = "00000000000000000000000000000000"

func main() {
	listen := flag.String("listen", defaultListen, "监听地址")
	cf := flag.String("counter", "stats/requests.count", "请求计数持久化文件")
	upstream := flag.String("gravatar-upstream", "https://secure.gravatar.com", "gravatar 源(被墙节点设为硅谷中转 https://gravatar-us.bluecdn.com)")
	cd := flag.String("cache-dir", "cache", "AVIF 缓存目录")
	sd := flag.String("site-dir", ".", "站点根目录(index.html 与 public/)")
	q := flag.Int("avif-quality", 55, "AVIF 质量 0-100")
	ttl := flag.Duration("cache-ttl", 7*24*time.Hour, "缓存有效期(过期重新回源),如 168h / 720h")
	ef := flag.String("esa-counter", "stats/esa.count", "阿里云 ESA 累计请求数文件")
	flag.Parse()
	counterFile = *cf
	cacheDir = *cd
	siteDir = *sd
	avifQuality = *q
	cacheTTL = *ttl
	esaFile = *ef

	emailSources = configuredEmailSources(*upstream)

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		log.Printf("warn: 无法创建缓存目录 %s: %v", cacheDir, err)
	}
	loadCounter()
	go persistCounterLoop()
	go cleanupLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/avatar/", avatarHandler)
	// /stats.php 旧路径保留兼容(重定向到 /stats)，避免旧书签/前端 404。
	mux.HandleFunc("/stats.php", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/stats", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/stats", statsHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, "ok")
	})
	// 站点静态资源(favicon/manifest)与首页由 go 自己服务，兜底 "/" 必须最后注册。
	mux.HandleFunc("/", siteHandler)

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("LiteAvatar on %s | gravatar=%s | cache=%s ttl=%s", *listen, emailSources[0].base, cacheDir, cacheTTL)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func configuredEmailSources(gravatarUpstream string) []source {
	return []source{
		{"gravatar", strings.TrimRight(gravatarUpstream, "/")},
		{"cravatar", "https://cravatar.com"},
		{"weavatar", "https://weavatar.com"},
		{"cnavatar", "https://cnavatar.com"},
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

	size := requestedSize(r)

	switch {
	case qqRe.MatchString(id):
		serveAvatar(w, r, fmt.Sprintf("qq:%s:%d", id, size), size, func(ctx context.Context) ([]byte, string, string, error) {
			return fetchQQ(ctx, id, size)
		})
	case emailHashRe.MatchString(id):
		serveAvatar(w, r, fmt.Sprintf("email:%s:%d", id, size), size, func(ctx context.Context) ([]byte, string, string, error) {
			return fetchGravatar(ctx, id, size, r.URL.Query().Get("d"))
		})
	default:
		writeDefault(w, r, size)
	}
}

// serveAvatar 缓存命中且未过期则按 Accept 返回；否则回源、转 AVIF 落盘、再返回。
func serveAvatar(w http.ResponseWriter, r *http.Request, key string, size int, fetch func(context.Context) ([]byte, string, string, error)) {
	avifPath, origPath, ctPath := cachePaths(key)

	// 缓存命中且未过期
	if !wantsRefresh(r) {
		if info, err := os.Stat(avifPath); err == nil && time.Since(info.ModTime()) < cacheTTL {
			if avifData, err := os.ReadFile(avifPath); err == nil {
				if acceptsAVIF(r) {
					output(w, avifData, "image/avif", "cache", "HIT")
					return
				}
				if orig, err := os.ReadFile(origPath); err == nil {
					output(w, orig, readCT(ctPath), "cache", "HIT")
					return
				}
			}
		}
	}

	// 未命中/过期：回源
	body, ct, src, err := fetch(r.Context())
	if err != nil || len(body) == 0 {
		writeDefault(w, r, size)
		return
	}
	body, ct, err = normalizeImageSize(body, ct, size)
	if err != nil || len(body) == 0 {
		writeDefault(w, r, size)
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

// fetchGravatar 严格按 emailSources 顺序串行回源，第一个有真实头像(d=404 下返回图片)的即采用。
// 某些兼容源会在 d=404 时仍返回占位图；这些已知占位图会被跳过，最后才用官方 Gravatar 默认图兜底。
func fetchGravatar(ctx context.Context, hash string, size int, d string) ([]byte, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()
	for _, s := range emailSources {
		url := fmt.Sprintf("%s/avatar/%s?s=%d&d=404", s.base, hash, size)
		body, ct, err := httpGet(ctx, url)
		if err == nil && len(body) > 0 {
			if isKnownPlaceholder(ctx, s, body, size) {
				continue
			}
			return body, ct, s.name, nil // 命中真实头像
		}
		if ctx.Err() != nil {
			break
		}
	}

	d = strings.TrimSpace(d)
	if d != "" && !strings.EqualFold(d, "404") {
		body, ct, err := httpGet(ctx, fmt.Sprintf("%s/avatar/%s?s=%d&d=%s", emailSources[0].base, hash, size, url.QueryEscape(d)))
		if err == nil && len(body) > 0 {
			return body, ct, "gravatar-default", nil
		}
	}
	return nil, "", "", errNoSource
}

// fetchQQ 拉取腾讯 QQ 头像。
func fetchQQ(ctx context.Context, qq string, size int) ([]byte, string, string, error) {
	url := fmt.Sprintf("https://q.qlogo.cn/headimg_dl?dst_uin=%s&spec=%d&img_type=jpg", qq, pickQQSpec(size))
	body, ct, err := httpGet(ctx, url)
	return body, ct, "qq", err
}

// httpGet 回源拉取图片，返回 body 与 Content-Type；非 200(含 404)视为该源无头像。
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

// toAVIF 解码原图(jpeg/png/gif/webp/avif)并编码为 AVIF。
func toAVIF(data []byte) ([]byte, error) {
	img, _, err := decodeImage(data, "")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := avif.Encode(&buf, img, avif.Options{Quality: avifQuality, Speed: 8}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// normalizeImageSize 保证所有上游都严格返回请求的正方形尺寸。
// QQ 只提供 40/100/140/640 四档，因此需要在本地缩放；其它源若尺寸正确则保留原始字节。
func normalizeImageSize(data []byte, contentType string, size int) ([]byte, string, error) {
	img, format, err := decodeImage(data, contentType)
	if err != nil {
		return nil, "", err
	}
	b := img.Bounds()
	if b.Dx() == size && b.Dy() == size {
		return data, canonicalImageContentType(format, contentType), nil
	}

	resized := resizeSquare(img, size)
	var buf bytes.Buffer
	if format == "jpeg" || format == "jpg" {
		if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/jpeg", nil
	}
	if err := png.Encode(&buf, resized); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/png", nil
}

func canonicalImageContentType(format, fallback string) string {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "avif":
		return "image/avif"
	default:
		return fallback
	}
}

func decodeImage(data []byte, contentType string) (image.Image, string, error) {
	if strings.Contains(strings.ToLower(contentType), "image/avif") {
		img, err := avif.Decode(bytes.NewReader(data))
		return img, "avif", err
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		return img, format, nil
	}
	img, avifErr := avif.Decode(bytes.NewReader(data))
	if avifErr == nil {
		return img, "avif", nil
	}
	return nil, "", err
}

func resizeSquare(src image.Image, size int) image.Image {
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	srcRect := image.Rect(x0, y0, x0+side, y0+side)
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, srcRect, xdraw.Src, nil)
	return dst
}

func acceptsAVIF(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "image/avif")
}

func wantsRefresh(r *http.Request) bool {
	v := strings.ToLower(r.URL.Query().Get("refresh"))
	return v == "1" || v == "true" || v == "yes"
}

// CNAvatar 当前会忽略 d=404，并为不存在的哈希返回自己的占位图。
// 每个请求尺寸首次使用时拉取同尺寸哨兵图并缓存哈希，避免把占位图误判为真实头像。
func isKnownPlaceholder(ctx context.Context, s source, body []byte, size int) bool {
	if s.name != "cnavatar" {
		return false
	}
	key := fmt.Sprintf("%s:%d", s.base, size)
	expected, ok := cnavatarPlaceholderHashes.Load(key)
	if !ok {
		probeURL := fmt.Sprintf("%s/avatar/%s?s=%d&d=404", s.base, cnavatarProbeHash, size)
		probe, _, err := httpGet(ctx, probeURL)
		if err != nil || len(probe) == 0 {
			return true // 无法确认不是占位图时，安全地跳过该源。
		}
		sum := sha1.Sum(probe)
		expected, _ = cnavatarPlaceholderHashes.LoadOrStore(key, sum)
	}
	return sha1.Sum(body) == expected.([sha1.Size]byte)
}

// output 输出图片，带缓存头、来源标记、缓存命中标记，并声明 Vary: Accept 供 CDN 分缓存。
func output(w http.ResponseWriter, body []byte, contentType, source, cache string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", sharedCacheControl(cacheTTL))
	w.Header().Set("Vary", "Accept")
	w.Header().Set("X-Avatar-Source", source)
	w.Header().Set("X-Cache", cache)
	w.Header().Set("X-Cache-Status", cache)
	w.Write(body)
}

func writeDefault(w http.ResponseWriter, r *http.Request, size int) {
	body, contentType, err := renderDefaultAvatar(size, acceptsAVIF(r))
	if err != nil {
		body = defaultAvatar
		contentType = "image/avif"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", sharedCacheControl(time.Hour))
	w.Header().Set("Vary", "Accept")
	w.Header().Set("X-Avatar-Source", "default")
	w.Write(body)
}

// sharedCacheControl keeps the browser copy immediately stale so a new page load
// reaches ESA and is visible in the request counter. Shared caches may still serve
// the stored object for ttl, so these revalidations do not become origin traffic.
func sharedCacheControl(ttl time.Duration) string {
	return fmt.Sprintf("public, max-age=0, must-revalidate, s-maxage=%d", int(ttl.Seconds()))
}

func renderDefaultAvatar(size int, wantAVIF bool) ([]byte, string, error) {
	contentType := "image/png"
	format := "png"
	if wantAVIF {
		contentType = "image/avif"
		format = "avif"
	}
	key := fmt.Sprintf("%d:%s", size, format)
	if body, ok := defaultCacheGet(key); ok {
		return body, contentType, nil
	}

	defaultAvatarOnce.Do(func() {
		defaultAvatarImage, defaultAvatarErr = avif.Decode(bytes.NewReader(defaultAvatar))
	})
	if defaultAvatarErr != nil {
		return nil, "", defaultAvatarErr
	}
	if wantAVIF && defaultAvatarImage.Bounds().Dx() == size && defaultAvatarImage.Bounds().Dy() == size {
		defaultCachePut(key, defaultAvatar)
		return defaultAvatar, contentType, nil
	}

	img := resizeSquare(defaultAvatarImage, size)
	var buf bytes.Buffer
	if wantAVIF {
		if err := avif.Encode(&buf, img, avif.Options{Quality: 90, Speed: 8}); err != nil {
			return nil, "", err
		}
	} else if err := png.Encode(&buf, img); err != nil {
		return nil, "", err
	}
	body := buf.Bytes()
	defaultCachePut(key, body)
	return body, contentType, nil
}

func defaultCacheGet(key string) ([]byte, bool) {
	defaultRenderCache.Lock()
	defer defaultRenderCache.Unlock()
	body, ok := defaultRenderCache.entries[key]
	return body, ok
}

func defaultCachePut(key string, body []byte) {
	defaultRenderCache.Lock()
	defer defaultRenderCache.Unlock()
	if _, ok := defaultRenderCache.entries[key]; ok {
		return
	}
	const maxEntries = 32
	if len(defaultRenderCache.order) >= maxEntries {
		oldest := defaultRenderCache.order[0]
		defaultRenderCache.order = defaultRenderCache.order[1:]
		delete(defaultRenderCache.entries, oldest)
	}
	defaultRenderCache.entries[key] = body
	defaultRenderCache.order = append(defaultRenderCache.order, key)
}

// ---- 缓存辅助 ----

func cachePaths(key string) (avifPath, origPath, ctPath string) {
	sum := sha1.Sum([]byte(cacheVersion + ":" + key))
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

// cleanupLoop 定期清理超过 cacheTTL 未更新的缓存文件。
func cleanupLoop() {
	t := time.NewTicker(6 * time.Hour)
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

func requestedSize(r *http.Request) int {
	s := r.URL.Query().Get("s")
	if s == "" {
		s = r.URL.Query().Get("size")
	}
	return parseSize(s)
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

// statsHandler 返回页脚统计。ESA 文件是有效头像 CDN 请求的唯一线上口径；
// local 是源站本地计数，仅在 ESA 尚无数据时作为降级值。
//
// 早先还有 Bunny / 百度两个来源，CDN 换成阿里云 ESA 后它们的抓取 timer 就已停用，
// 字段长期恒为 0，于 2026-08-02 一并移除（前端只读 requests，不受影响）。
func statsHandler(w http.ResponseWriter, _ *http.Request) {
	local := atomic.LoadInt64(&requestCount)
	esa := readCounterFile(esaFile)
	total := esa
	if total == 0 {
		total = local
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, `{"requests":%d,"local":%d,"esa":%d}`, total, local, esa)
}

// readCounterFile 读取后台脚本定时写入的 CDN 累计请求数文件。
// go 自身不调 CDN API —— 由 systemd timer 每小时跑脚本拉取并落盘，规避 API 配额/限流，
// 且不受访问流量影响。文件不存在或读取失败返回 0。
func readCounterFile(path string) int64 {
	if path == "" {
		return 0
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// siteHandler 服务首页与站点静态资源(favicon/manifest 等)，从 -site-dir 读取。
func siteHandler(w http.ResponseWriter, r *http.Request) {
	// 根路径返回首页
	clean := strings.TrimPrefix(r.URL.Path, "/")
	if clean == "" || clean == "index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(readSiteFile("index.html"))
		return
	}
	// 其余按文件名从 public/ 取（favicon.ico、site.webmanifest 等）
	data, ok := trySiteFile(clean)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(clean))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// readSiteFile 从站点根目录读取文件，返回字节；找不到返回 nil。
func readSiteFile(name string) []byte {
	b, err := os.ReadFile(filepath.Join(siteDir, filepath.Base(name)))
	if err != nil {
		return nil
	}
	return b
}

func trySiteFile(name string) ([]byte, bool) {
	clean := filepath.Clean("/" + name)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "." || strings.HasPrefix(clean, "../") {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(siteDir, "public", clean))
	if err != nil {
		return nil, false
	}
	return b, true
}

// contentTypeFor 按扩展名返回常见静态资源的 Content-Type。
func contentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".ico":
		return "image/x-icon"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".json", ".webmanifest":
		return "application/json; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
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
	os.MkdirAll(filepath.Dir(counterFile), 0o755)
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
