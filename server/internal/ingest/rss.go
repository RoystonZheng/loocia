package ingest

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
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
	return parseFeedWithOptions(data, sourceName, sourceKind, SourceRoleDiscovery, 0)
}

func parseFeedWithOptions(data []byte, sourceName, sourceKind, sourceRole string, maxItems int) ([]RawItem, error) {
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
			SourceRole:  DefaultSourceRole(sourceRole),
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
		if maxItems > 0 && len(out) >= maxItems {
			break
		}
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
	name       string
	feedURL    string
	sourceKind string
	sourceRole string
	maxItems   int
	attempts   int
	client     *http.Client
}

func NewRSSSource(name, feedURL string) *RSSSource {
	return NewRSSSourceWithOptions(name, feedURL, RSSSourceOptions{})
}

type RSSSourceOptions struct {
	SourceKind     string
	SourceRole     string
	MaxItemsPerRun int
	Timeout        time.Duration
	RetryAttempts  int
}

func NewRSSSourceWithOptions(name, feedURL string, opts RSSSourceOptions) *RSSSource {
	sourceKind := opts.SourceKind
	if sourceKind == "" {
		sourceKind = SourceKindRSS
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	attempts := opts.RetryAttempts
	if attempts <= 0 {
		attempts = 2
	}
	if attempts > 4 {
		attempts = 4
	}
	return &RSSSource{
		name:       name,
		feedURL:    feedURL,
		sourceKind: sourceKind,
		sourceRole: DefaultSourceRole(opts.SourceRole),
		maxItems:   opts.MaxItemsPerRun,
		attempts:   attempts,
		client:     newSourceHTTPClient(timeout),
	}
}

func (r *RSSSource) Name() string { return r.name }

func (r *RSSSource) Fetch(ctx context.Context) ([]RawItem, error) {
	var lastErr error
	for attempt := 1; attempt <= r.attempts; attempt++ {
		items, retry, retryAfter, err := r.fetchOnce(ctx)
		if err == nil {
			return items, nil
		}
		lastErr = err
		if !retry || attempt == r.attempts {
			return nil, err
		}
		if err := waitRSSRetry(ctx, rssRetryDelay(attempt, retryAfter)); err != nil {
			return nil, fmt.Errorf("fetch %q: retry wait: %w", r.name, err)
		}
	}
	return nil, lastErr
}

const (
	rssUserAgent = "aihot-ingest/0.2 (+https://aihot.virxact.com/)"
	rssAccept    = "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.1"
	rssRetryCap  = 3 * time.Second
	rssRetryBase = 250 * time.Millisecond
)

func (r *RSSSource) fetchOnce(ctx context.Context) ([]RawItem, bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.feedURL, nil)
	if err != nil {
		return nil, false, "", err
	}
	req.Header.Set("User-Agent", rssUserAgent)
	req.Header.Set("Accept", rssAccept)
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, true, "", fmt.Errorf("fetch %q: %w", r.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		retryAfter := resp.Header.Get("Retry-After")
		retryable := resp.StatusCode == http.StatusRequestTimeout ||
			resp.StatusCode == http.StatusTooEarly ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode >= http.StatusInternalServerError
		if retryAfter != "" {
			return nil, retryable, retryAfter, fmt.Errorf("fetch %q: status %d retry-after %s", r.name, resp.StatusCode, retryAfter)
		}
		return nil, retryable, "", fmt.Errorf("fetch %q: status %d", r.name, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, "", fmt.Errorf("read %q body: %w", r.name, err)
	}
	items, err := parseFeedWithOptions(body, r.name, r.sourceKind, r.sourceRole, r.maxItems)
	if err != nil {
		return nil, false, "", err
	}
	return items, false, "", nil
}

func rssRetryDelay(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		delay := time.Duration(seconds) * time.Second
		if delay > rssRetryCap {
			return rssRetryCap
		}
		return delay
	}
	delay := time.Duration(attempt) * rssRetryBase
	if delay > rssRetryCap {
		return rssRetryCap
	}
	return delay
}

func waitRSSRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
