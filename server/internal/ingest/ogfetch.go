package ingest

import (
	"context"
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
			return m[1]
		}
	}
	return ""
}

// FetchOGMedia fetches the article page and reads its Open Graph image/video
// tags. Best-effort: any error (network, non-200, no tags) yields empty strings.
// Only the head is scanned (og tags live there) to bound cost.
func FetchOGMedia(ctx context.Context, client *http.Client, pageURL string) (image, video string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; aihot-ingest/0.1)")
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", ""
	}
	html := string(body)
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

// OGResolver fills missing media from a page's Open Graph tags. It satisfies the
// pipeline's media-resolver hook.
type OGResolver struct{ client *http.Client }

func NewOGResolver() *OGResolver {
	return &OGResolver{client: &http.Client{Timeout: 10 * time.Second}}
}

// Resolve returns (image, video) pointers, nil when absent.
func (o *OGResolver) Resolve(ctx context.Context, pageURL string) (image, video *string) {
	i, v := FetchOGMedia(ctx, o.client, pageURL)
	if i != "" {
		image = &i
	}
	if v != "" {
		video = &v
	}
	return image, video
}
