package ingest

import (
	"bytes"
	"net/url"
	"strings"

	readability "github.com/go-shiori/go-readability"
)

// extractArticle runs Mozilla-Readability over a page's HTML and returns the
// main-content HTML. ok=false on parse failure or trivially small content.
func extractArticle(htmlBytes []byte, pageURL string) (string, bool) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", false
	}
	art, err := readability.FromReader(bytes.NewReader(htmlBytes), u)
	if err != nil {
		return "", false
	}
	if len(strings.TrimSpace(art.TextContent)) < 200 {
		return "", false
	}
	return strings.TrimSpace(art.Content), true
}
