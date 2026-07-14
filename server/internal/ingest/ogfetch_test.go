package ingest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOGMediaFrom(t *testing.T) {
	const page = `<!doctype html><html><head>
<meta charset="utf-8">
<meta property="og:image" content="https://cdn.ex.com/og.jpg">
<meta property="og:video:secure_url" content="https://cdn.ex.com/og.mp4">
<title>x</title></head><body>hi</body></html>`
	img, vid := ogMediaFrom(page)
	if img != "https://cdn.ex.com/og.jpg" {
		t.Fatalf("image: %q", img)
	}
	if vid != "https://cdn.ex.com/og.mp4" {
		t.Fatalf("video: %q", vid)
	}
}

func TestOGMediaFromReversedAttrOrder(t *testing.T) {
	const page = `<html><head>
<meta content="https://cdn.ex.com/tw.jpg" name="twitter:image">
</head></html>`
	img, vid := ogMediaFrom(page)
	if img != "https://cdn.ex.com/tw.jpg" {
		t.Fatalf("image: %q", img)
	}
	if vid != "" {
		t.Fatalf("video should be empty, got %q", vid)
	}
}

func TestOGMediaFromDecodesEntities(t *testing.T) {
	// og content is raw HTML source: query separators arrive as &amp;.
	const page = `<html><head>
<meta property="og:video" content="https://player.ex.com/v/1?a=1&amp;b=2">
</head></html>`
	_, vid := ogMediaFrom(page)
	if vid != "https://player.ex.com/v/1?a=1&b=2" {
		t.Fatalf("entities not decoded: %q", vid)
	}
}

func TestOGMediaFromNoTags(t *testing.T) {
	img, vid := ogMediaFrom(`<html><head><title>plain</title></head></html>`)
	if img != "" || vid != "" {
		t.Fatalf("want empty, got img=%q vid=%q", img, vid)
	}
}

func TestPageResolverNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewPageResolver()
	img, vid, article := r.Resolve(context.Background(), srv.URL)
	if img != nil || vid != nil || article != nil {
		t.Fatalf("want all nil on 404, got img=%v vid=%v article=%v", img, vid, article)
	}
}

func TestPageResolverReturnsMediaAndArticle(t *testing.T) {
	page := `<html><head><meta property="og:image" content="https://cdn.example.com/x.jpg"></head><body>` +
		`<article><h1>Headline</h1><p>` + strings.Repeat("real article words here that form a substantial body of readable content ", 20) + `</p></article></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, page)
	}))
	defer srv.Close()

	r := NewPageResolver()
	img, _, article := r.Resolve(context.Background(), srv.URL)
	if img == nil || *img != "https://cdn.example.com/x.jpg" {
		t.Fatalf("og image: %v", img)
	}
	if article == nil || !strings.Contains(*article, "real article words") {
		t.Fatalf("article: %v", article)
	}
}

func TestPageResolverBestEffortOnError(t *testing.T) {
	r := NewPageResolver()
	img, vid, article := r.Resolve(context.Background(), "http://127.0.0.1:1/nope")
	if img != nil || vid != nil || article != nil {
		t.Fatalf("all nil on fetch failure: %v %v %v", img, vid, article)
	}
}
