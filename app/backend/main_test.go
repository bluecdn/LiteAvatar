package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gen2brain/avif"
)

func TestConfiguredEmailSources(t *testing.T) {
	sources := configuredEmailSources("https://gravatar.example/")
	got := make([]string, 0, len(sources))
	for _, s := range sources {
		got = append(got, s.name+"="+s.base)
	}
	want := []string{
		"gravatar=https://gravatar.example",
		"cravatar=https://cravatar.com",
		"weavatar=https://weavatar.com",
		"cnavatar=https://cnavatar.com",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("configured sources = %v, want %v", got, want)
	}
}

func TestRequestedSizeSupportsAliasesAndBounds(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{"", 80},
		{"?s=64", 64},
		{"?size=96", 96},
		{"?s=32&size=96", 32},
		{"?s=0", 80},
		{"?s=4096", 2048},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/avatar/test"+tt.query, nil)
		if got := requestedSize(r); got != tt.want {
			t.Errorf("requestedSize(%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

func TestNormalizeImageSize(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			src.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 180, A: 255})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, src); err != nil {
		t.Fatal(err)
	}

	body, contentType, err := normalizeImageSize(input.Bytes(), "image/png", 64)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/png" {
		t.Fatalf("content type = %q", contentType)
	}
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got != (image.Point{X: 64, Y: 64}) {
		t.Fatalf("normalized size = %v, want 64x64", got)
	}
}

func TestNormalizeImageSizeCorrectsWrongContentType(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 80, 80))
	var input bytes.Buffer
	if err := jpeg.Encode(&input, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	body, contentType, err := normalizeImageSize(input.Bytes(), "image/png", 80)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", contentType)
	}
	if !bytes.Equal(body, input.Bytes()) {
		t.Fatal("correctly sized image was unnecessarily re-encoded")
	}
}

func TestDefaultAvatarRendersExactSize(t *testing.T) {
	for _, tc := range []struct {
		avif        bool
		contentType string
	}{
		{false, "image/png"},
		{true, "image/avif"},
	} {
		body, contentType, err := renderDefaultAvatar(73, tc.avif)
		if err != nil {
			t.Fatal(err)
		}
		if contentType != tc.contentType {
			t.Fatalf("content type = %q, want %q", contentType, tc.contentType)
		}
		var img image.Image
		if tc.avif {
			img, err = avif.Decode(bytes.NewReader(body))
		} else {
			img, _, err = image.Decode(bytes.NewReader(body))
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := img.Bounds().Size(); got != (image.Point{X: 73, Y: 73}) {
			t.Fatalf("default size = %v, want 73x73", got)
		}
		r, g, b, _ := img.At(0, 0).RGBA()
		if b <= r || b <= g {
			t.Fatalf("default corner is not blue: r=%d g=%d b=%d", r, g, b)
		}
	}
}

func TestCNAvatarPlaceholderProbe(t *testing.T) {
	const placeholder = "same-size-placeholder"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		if r.URL.Path == "/avatar/realhash" {
			_, _ = w.Write([]byte("real-avatar"))
			return
		}
		_, _ = w.Write([]byte(placeholder))
	}))
	defer server.Close()

	s := source{name: "cnavatar", base: server.URL}
	if !isKnownPlaceholder(context.Background(), s, []byte(placeholder), 64) {
		t.Fatal("CNAvatar placeholder was treated as a real avatar")
	}
	if isKnownPlaceholder(context.Background(), s, []byte("real-avatar"), 64) {
		t.Fatal("real CNAvatar image was treated as a placeholder")
	}
}

func TestInvalidIDUsesSizedDefault(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/avatar/not-a-hash?size=61", nil)
	r.Header.Set("Accept", "image/avif")
	w := httptest.NewRecorder()
	avatarHandler(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Avatar-Source"); got != "default" {
		t.Fatalf("source = %q", got)
	}
	img, err := avif.Decode(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got != (image.Point{X: 61, Y: 61}) {
		t.Fatalf("default size = %v, want 61x61", got)
	}
}

func TestSharedCacheControlRevalidatesBrowserAndCachesAtEdge(t *testing.T) {
	got := sharedCacheControl(7 * 24 * time.Hour)
	want := "public, max-age=0, must-revalidate, s-maxage=604800"
	if got != want {
		t.Fatalf("Cache-Control = %q, want %q", got, want)
	}
}

func TestStatsUsesESAAsAuthoritativeMetric(t *testing.T) {
	dir := t.TempDir()
	oldESA, oldBunny, oldBaidu := esaFile, bunnyFile, baiduFile
	oldLocal := atomic.LoadInt64(&requestCount)
	t.Cleanup(func() {
		esaFile, bunnyFile, baiduFile = oldESA, oldBunny, oldBaidu
		atomic.StoreInt64(&requestCount, oldLocal)
	})

	esaFile = filepath.Join(dir, "esa.count")
	bunnyFile = filepath.Join(dir, "bunny.count")
	baiduFile = filepath.Join(dir, "baidu.count")
	if err := os.WriteFile(esaFile, []byte("100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bunnyFile, []byte("500\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	atomic.StoreInt64(&requestCount, 1000)

	w := httptest.NewRecorder()
	statsHandler(w, httptest.NewRequest(http.MethodGet, "/stats", nil))
	if got, want := w.Body.String(), `{"requests":100,"local":1000,"esa":100,"bunny":500,"baidu":0}`; got != want {
		t.Fatalf("stats body = %s, want %s", got, want)
	}
}
