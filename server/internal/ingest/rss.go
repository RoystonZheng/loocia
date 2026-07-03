package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"
)

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
		out = append(out, r)
	}
	return out, nil
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
