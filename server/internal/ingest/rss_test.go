package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const sampleRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <title>Example AI Blog</title>
  <item>
    <title>Model X released</title>
    <link>https://ex.com/model-x</link>
    <description>We shipped Model X.</description>
    <pubDate>Wed, 07 May 2026 12:00:00 GMT</pubDate>
  </item>
  <item>
    <title>No date item</title>
    <link>https://ex.com/no-date</link>
    <description>Body two.</description>
  </item>
  <item>
    <title>Empty link item</title>
    <link></link>
    <description>should be skipped</description>
  </item>
</channel></rss>`

func TestParseFeedMapsItems(t *testing.T) {
	items, err := parseFeed([]byte(sampleRSS), "Example AI Blog", "rss")
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	// Empty-link item is skipped → 2 items.
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}

	first := items[0]
	if first.Title != "Model X released" || first.URL != "https://ex.com/model-x" {
		t.Fatalf("item0 fields: %+v", first)
	}
	if first.ID != RawID("https://ex.com/model-x") {
		t.Fatalf("item0 id not derived from URL: %s", first.ID)
	}
	if first.Source != "Example AI Blog" || first.SourceKind != "rss" {
		t.Fatalf("item0 source/kind: %+v", first)
	}
	if first.PublishedAt == nil || !first.PublishedAt.Equal(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("item0 published_at: %v", first.PublishedAt)
	}
	if first.RawContent == nil || *first.RawContent != "We shipped Model X." {
		t.Fatalf("item0 raw_content: %v", first.RawContent)
	}

	// Second item has no pubDate → PublishedAt nil.
	if items[1].PublishedAt != nil {
		t.Fatalf("item1 should have nil PublishedAt, got %v", items[1].PublishedAt)
	}
}

func TestRSSSourceFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(sampleRSS))
	}))
	defer srv.Close()

	src := NewRSSSource("Example AI Blog", srv.URL)
	if src.Name() != "Example AI Blog" {
		t.Fatalf("Name: %q", src.Name())
	}
	items, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items from Fetch, got %d", len(items))
	}
}
