package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type HTMLSourceOptions struct {
	SourceKind     string
	SourceRole     string
	MaxItemsPerRun int
	Timeout        time.Duration
}

type AnthropicNewsSource struct {
	name       string
	pageURL    string
	sourceKind string
	sourceRole string
	maxItems   int
	client     *http.Client
}

func NewAnthropicNewsSource(name, pageURL string, opts HTMLSourceOptions) *AnthropicNewsSource {
	sourceKind := opts.SourceKind
	if sourceKind == "" {
		sourceKind = SourceKindHTML
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &AnthropicNewsSource{
		name:       name,
		pageURL:    pageURL,
		sourceKind: sourceKind,
		sourceRole: DefaultSourceRole(opts.SourceRole),
		maxItems:   opts.MaxItemsPerRun,
		client:     &http.Client{Timeout: timeout},
	}
}

func (s *AnthropicNewsSource) Name() string { return s.name }

func (s *AnthropicNewsSource) Fetch(ctx context.Context) ([]RawItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "aihot-ingest/0.1")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", s.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			return nil, fmt.Errorf("fetch %q: status %d retry-after %s", s.name, resp.StatusCode, retryAfter)
		}
		return nil, fmt.Errorf("fetch %q: status %d", s.name, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %q body: %w", s.name, err)
	}
	return parseAnthropicNewsHTML(body, s.pageURL, s.name, s.sourceKind, s.sourceRole, s.maxItems)
}

func parseAnthropicNewsHTML(data []byte, pageURL, sourceName, sourceKind, sourceRole string, maxItems int) ([]RawItem, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("parse html %q: %w", sourceName, err)
	}
	seen := map[string]bool{}
	var out []RawItem
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		if maxItems > 0 && len(out) >= maxItems {
			return
		}
		href, _ := a.Attr("href")
		u, err := base.Parse(strings.TrimSpace(href))
		if err != nil || u.Scheme == "" || u.Host != base.Host || !strings.HasPrefix(u.Path, "/news/") {
			return
		}
		u.Fragment = ""
		link := u.String()
		if seen[link] {
			return
		}
		title := strings.Join(strings.Fields(a.Text()), " ")
		if title == "" {
			return
		}
		seen[link] = true
		item := RawItem{
			ID:         RawID(link),
			Source:     sourceName,
			SourceKind: sourceKind,
			SourceRole: DefaultSourceRole(sourceRole),
			URL:        link,
			Title:      title,
		}
		if published := closestTime(a); published != nil {
			item.PublishedAt = published
		}
		out = append(out, item)
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("parse html %q: no news links found", sourceName)
	}
	return out, nil
}

func closestTime(s *goquery.Selection) *time.Time {
	container := s.Closest("article,li,section,div")
	if container.Length() == 0 {
		container = s.Parent()
	}
	if datetime, ok := container.Find("time").First().Attr("datetime"); ok {
		if ts := parseOptionalTime(datetime); ts != nil {
			return ts
		}
	}
	return nil
}

func parseOptionalTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return &t
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return &t
	}
	return nil
}
