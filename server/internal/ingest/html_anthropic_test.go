package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const anthropicHTML = `<!doctype html><html><body>
<main>
  <article>
    <a href="/news/claude-example">Claude example release</a>
    <time datetime="2026-09-12T08:30:00Z">Sep 12</time>
  </article>
  <article>
    <a href="https://www.anthropic.com/news/research-update">Research update</a>
    <time datetime="2026-09-10">Sep 10</time>
  </article>
  <a href="/company">Company</a>
</main>
</body></html>`

func TestParseAnthropicNewsHTML(t *testing.T) {
	items, err := parseAnthropicNewsHTML([]byte(anthropicHTML), "https://www.anthropic.com/news", "Anthropic News", SourceKindHTML, SourceRoleOfficial, 10)
	if err != nil {
		t.Fatalf("parseAnthropicNewsHTML: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 news items, got %d", len(items))
	}
	if items[0].URL != "https://www.anthropic.com/news/claude-example" || items[0].Title != "Claude example release" {
		t.Fatalf("item0: %+v", items[0])
	}
	if items[0].SourceKind != SourceKindHTML || items[0].SourceRole != SourceRoleOfficial {
		t.Fatalf("source fields: %+v", items[0])
	}
	if items[0].PublishedAt == nil || !items[0].PublishedAt.Equal(time.Date(2026, 9, 12, 8, 30, 0, 0, time.UTC)) {
		t.Fatalf("published: %v", items[0].PublishedAt)
	}
}

func TestParseAnthropicNewsHTMLLimitAndNoLinks(t *testing.T) {
	items, err := parseAnthropicNewsHTML([]byte(anthropicHTML), "https://www.anthropic.com/news", "Anthropic News", SourceKindHTML, SourceRoleOfficial, 1)
	if err != nil {
		t.Fatalf("parseAnthropicNewsHTML: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 limited item, got %d", len(items))
	}
	if _, err := parseAnthropicNewsHTML([]byte(`<a href="/company">Company</a>`), "https://www.anthropic.com/news", "Anthropic News", SourceKindHTML, SourceRoleOfficial, 10); err == nil {
		t.Fatal("missing news links should return an error")
	}
}

func TestAnthropicNewsSourceFetch(t *testing.T) {
	var userAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(anthropicHTML))
	}))
	defer srv.Close()

	src := NewAnthropicNewsSource("Anthropic News", srv.URL, HTMLSourceOptions{SourceRole: SourceRoleOfficial})
	items, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 same-host relative item, got %d", len(items))
	}
	if userAgent != "aihot-ingest/0.1" {
		t.Fatalf("user-agent: %q", userAgent)
	}
}

func TestAnthropicNewsSourceReportsRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := NewAnthropicNewsSource("Anthropic News", srv.URL, HTMLSourceOptions{}).Fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "retry-after 30") {
		t.Fatalf("expected retry-after error, got %v", err)
	}
}
