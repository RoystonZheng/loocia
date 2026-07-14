package ingest

import (
	"strings"
	"testing"
)

func TestExtractArticlePullsMainContent(t *testing.T) {
	html := `<html><head><title>T</title></head><body>
	<nav>menu home about contact</nav>
	<article><h1>Big News</h1><p>This is the first substantial paragraph of the real article body with enough words to be considered content by the readability algorithm, describing what actually happened in some detail here.</p><p>A second paragraph continues the story with more concrete detail and context so the extractor clearly identifies this block as the main readable content of the page.</p></article>
	<aside>advertisement sidebar junk links</aside></body></html>`
	out, ok := extractArticle([]byte(html), "https://example.com/a")
	if !ok {
		t.Fatal("should extract main content")
	}
	if !strings.Contains(out, "substantial paragraph") || !strings.Contains(out, "second paragraph") {
		t.Fatalf("main content missing: %q", out)
	}
	if strings.Contains(out, "advertisement sidebar junk") {
		t.Fatalf("noise not stripped: %q", out)
	}
}

func TestExtractArticleEmptyOnGarbage(t *testing.T) {
	if _, ok := extractArticle([]byte("<html><body></body></html>"), "https://e.com/x"); ok {
		t.Fatal("empty page should not extract")
	}
}
