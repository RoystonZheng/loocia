package ingest

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

// imgSrcRe pulls the src of the first <img> in a chunk of feed HTML.
var imgSrcRe = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)

// videoSrcRe matches a <video src=…> or an embed <iframe src=…> (youtube/vimeo/etc).
var videoSrcRe = regexp.MustCompile(`(?i)<(?:video|iframe|source)[^>]+src=["']([^"']+)["']`)

// parseFeed turns raw RSS/Atom bytes into RawItems. Items without a link are
// skipped (no stable id derivable).
func parseFeed(data []byte, sourceName, sourceKind string) ([]RawItem, error) {
	feed, err := gofeed.NewParser().ParseString(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse feed %q: %w", sourceName, err)
	}
	var out []RawItem
	for _, it := range feed.Items {
		if it.Link == "" {
			continue
		}
		r := RawItem{
			ID:          RawID(it.Link),
			Source:      sourceName,
			SourceKind:  sourceKind,
			URL:         it.Link,
			Title:       it.Title,
			PublishedAt: it.PublishedParsed,
		}
		if body := firstNonEmpty(it.Description, it.Content); body != "" {
			r.RawContent = &body
		}
		if img := extractImage(it); img != "" {
			r.ImageURL = &img
		}
		if vid := extractVideo(it); vid != "" {
			r.VideoURL = &vid
		}
		out = append(out, r)
	}
	return out, nil
}

// extractImage returns a representative http(s) image URL for the item, or "".
// Order: a feed-provided <image>/media thumbnail, then the first image
// enclosure, then the first <img> in the content/description HTML — where blog
// feeds (our main image source; RSS media tags are rare here) put the hero image.
func extractImage(it *gofeed.Item) string {
	if it.Image != nil && strings.HasPrefix(it.Image.URL, "http") {
		return it.Image.URL
	}
	for _, e := range it.Enclosures {
		if e != nil && strings.HasPrefix(e.Type, "image/") && strings.HasPrefix(e.URL, "http") {
			return e.URL
		}
	}
	for _, h := range []string{it.Content, it.Description} {
		if m := imgSrcRe.FindStringSubmatch(h); m != nil && strings.HasPrefix(m[1], "http") {
			// The src comes from raw HTML source, so it is entity-encoded
			// (e.g. query separators as &amp;); decode before storing.
			return html.UnescapeString(m[1])
		}
	}
	return ""
}

// extractVideo returns a video/embed URL for the item, or "". Order: an image
// enclosure's video sibling (media/enclosure of a video type), then a <video>/
// <iframe> src in the content. og:video fallback (network) is handled separately.
func extractVideo(it *gofeed.Item) string {
	for _, e := range it.Enclosures {
		if e != nil && strings.HasPrefix(e.Type, "video/") && strings.HasPrefix(e.URL, "http") {
			return e.URL
		}
	}
	for _, h := range []string{it.Content, it.Description} {
		if m := videoSrcRe.FindStringSubmatch(h); m != nil && strings.HasPrefix(m[1], "http") {
			return html.UnescapeString(m[1])
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// RSSSource fetches and parses one RSS/Atom feed.
type RSSSource struct {
	name    string
	feedURL string
	client  *http.Client
}

func NewRSSSource(name, feedURL string) *RSSSource {
	return &RSSSource{name: name, feedURL: feedURL, client: &http.Client{Timeout: 30 * time.Second}}
}

func (r *RSSSource) Name() string { return r.name }

func (r *RSSSource) Fetch(ctx context.Context) ([]RawItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "aihot-ingest/0.1")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", r.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %q: status %d", r.name, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %q body: %w", r.name, err)
	}
	return parseFeed(body, r.name, "rss")
}
