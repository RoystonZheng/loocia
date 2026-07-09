package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchOGMedia(t *testing.T) {
	const page = `<!doctype html><html><head>
<meta charset="utf-8">
<meta property="og:image" content="https://cdn.ex.com/og.jpg">
<meta property="og:video:secure_url" content="https://cdn.ex.com/og.mp4">
<title>x</title></head><body>hi</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(page))
	}))
	defer srv.Close()

	img, vid := FetchOGMedia(context.Background(), srv.Client(), srv.URL)
	if img != "https://cdn.ex.com/og.jpg" {
		t.Fatalf("image: %q", img)
	}
	if vid != "https://cdn.ex.com/og.mp4" {
		t.Fatalf("video: %q", vid)
	}
}

func TestFetchOGMediaReversedAttrOrder(t *testing.T) {
	const page = `<html><head>
<meta content="https://cdn.ex.com/tw.jpg" name="twitter:image">
</head></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(page))
	}))
	defer srv.Close()

	img, vid := FetchOGMedia(context.Background(), srv.Client(), srv.URL)
	if img != "https://cdn.ex.com/tw.jpg" {
		t.Fatalf("image: %q", img)
	}
	if vid != "" {
		t.Fatalf("video should be empty, got %q", vid)
	}
}

func TestFetchOGMediaNoTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html><head><title>plain</title></head></html>`))
	}))
	defer srv.Close()

	img, vid := FetchOGMedia(context.Background(), srv.Client(), srv.URL)
	if img != "" || vid != "" {
		t.Fatalf("want empty, got img=%q vid=%q", img, vid)
	}
}

func TestFetchOGMediaNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	img, vid := FetchOGMedia(context.Background(), srv.Client(), srv.URL)
	if img != "" || vid != "" {
		t.Fatalf("want empty on 404, got img=%q vid=%q", img, vid)
	}
}
