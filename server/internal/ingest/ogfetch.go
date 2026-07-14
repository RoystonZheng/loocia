package ingest

import (
	"context"
	htmlpkg "html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ogContentRe[prop] matches <meta property="prop" ... content="X"> in either
// attribute order. Built once for the handful of properties we read.
var ogContentRe = func() map[string][2]*regexp.Regexp {
	props := []string{"og:image", "twitter:image", "og:video:secure_url", "og:video:url", "og:video", "twitter:player"}
	m := map[string][2]*regexp.Regexp{}
	for _, p := range props {
		q := regexp.QuoteMeta(p)
		m[p] = [2]*regexp.Regexp{
			regexp.MustCompile(`(?i)<meta[^>]+(?:property|name)=["']` + q + `["'][^>]*content=["']([^"']+)["']`),
			regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]*(?:property|name)=["']` + q + `["']`),
		}
	}
	return m
}()

func ogContent(html, prop string) string {
	res, ok := ogContentRe[prop]
	if !ok {
		return ""
	}
	for _, re := range res {
		if m := re.FindStringSubmatch(html); m != nil {
			// content="" is raw HTML source: entity-decode (e.g. &amp; → &)
			// so URL query separators survive round-tripping through templates.
			return htmlpkg.UnescapeString(m[1])
		}
	}
	return ""
}

const maxPageBytes = 3 << 20

// fetchPage GETs a page and returns its body bytes (capped). ok=false on any
// error / non-200. Best-effort.
func fetchPage(ctx context.Context, client *http.Client, pageURL string) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; aihot-ingest/0.1)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		return nil, false
	}
	return b, true
}

// ogMediaFrom extracts og image/video URLs from page HTML (empty when absent/non-http).
func ogMediaFrom(html string) (image, video string) {
	image = firstNonEmpty(ogContent(html, "og:image"), ogContent(html, "twitter:image"))
	video = firstNonEmpty(ogContent(html, "og:video:secure_url"), ogContent(html, "og:video:url"), ogContent(html, "og:video"), ogContent(html, "twitter:player"))
	if !strings.HasPrefix(image, "http") {
		image = ""
	}
	if !strings.HasPrefix(video, "http") {
		video = ""
	}
	return image, video
}

// PageResolver fetches an article page once and returns its OG media plus the
// readability-extracted main article HTML. All returns are best-effort (nil on
// miss). Satisfies the pipeline's page-resolver hook.
type PageResolver struct{ client *http.Client }

func NewPageResolver() *PageResolver {
	return &PageResolver{client: &http.Client{Timeout: 15 * time.Second}}
}

// Resolve returns (image, video, article) pointers, nil when absent.
func (p *PageResolver) Resolve(ctx context.Context, pageURL string) (image, video, article *string) {
	b, ok := fetchPage(ctx, p.client, pageURL)
	if !ok {
		return nil, nil, nil
	}
	html := string(b)
	if i, v := ogMediaFrom(html); i != "" || v != "" {
		if i != "" {
			image = &i
		}
		if v != "" {
			video = &v
		}
	}
	if art, ok := extractArticle(b, pageURL); ok {
		article = &art
	}
	return image, video, article
}
