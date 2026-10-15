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

// HTMLListOptions maps a listing page to article links. Selectors are CSS
// selectors and can be adjusted per site without adding a site-specific Go
// adapter. The defaults work for common article/list/card markup.
type HTMLListOptions struct {
	SourceKind     string
	SourceRole     string
	ItemSelector   string
	LinkSelector   string
	TitleSelector  string
	DateSelector   string
	MaxItemsPerRun int
	Timeout        time.Duration
}

type HTMLListSource struct {
	name    string
	pageURL string
	options HTMLListOptions
	client  *http.Client
}

func NewHTMLListSource(name, pageURL string, opts HTMLListOptions) *HTMLListSource {
	if opts.SourceKind == "" {
		opts.SourceKind = SourceKindHTML
	}
	if opts.ItemSelector == "" {
		opts.ItemSelector = "article, main li, main .card, main .post, main .item"
	}
	if opts.LinkSelector == "" {
		opts.LinkSelector = "a[href]"
	}
	if opts.TitleSelector == "" {
		opts.TitleSelector = "h1, h2, h3, h4, [class*='title'], [class*='headline']"
	}
	if opts.DateSelector == "" {
		opts.DateSelector = "time, [datetime]"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	return &HTMLListSource{name: name, pageURL: pageURL, options: opts, client: newSourceHTTPClient(opts.Timeout)}
}

func (s *HTMLListSource) Name() string { return s.name }

func (s *HTMLListSource) Fetch(ctx context.Context) ([]RawItem, error) {
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %q: status %d", s.name, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read %q body: %w", s.name, err)
	}
	return parseHTMLList(body, s.pageURL, s.name, s.options)
}

func parseHTMLList(data []byte, pageURL, sourceName string, opts HTMLListOptions) ([]RawItem, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, fmt.Errorf("parse html base %q: %w", sourceName, err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("parse html %q: %w", sourceName, err)
	}
	seen := map[string]bool{}
	var out []RawItem
	selection := doc.Find(opts.ItemSelector)
	if selection.Length() == 0 {
		selection = doc.Find("a[href]")
	}
	selection.Each(func(_ int, node *goquery.Selection) {
		if opts.MaxItemsPerRun > 0 && len(out) >= opts.MaxItemsPerRun {
			return
		}
		linkNode := node.Find(opts.LinkSelector).First()
		if linkNode.Length() == 0 && node.Is("a[href]") {
			linkNode = node
		}
		href, ok := linkNode.Attr("href")
		if !ok || strings.TrimSpace(href) == "" {
			return
		}
		parsed, err := base.Parse(strings.TrimSpace(href))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host != base.Host {
			return
		}
		parsed.Fragment = ""
		link := parsed.String()
		if seen[link] {
			return
		}
		titleNode := node.Find(opts.TitleSelector).First()
		title := strings.TrimSpace(titleNode.Text())
		if title == "" {
			title = strings.TrimSpace(linkNode.Text())
		}
		title = strings.Join(strings.Fields(title), " ")
		if title == "" {
			return
		}
		item := RawItem{ID: RawID(link), Source: sourceName, SourceKind: opts.SourceKind, SourceRole: DefaultSourceRole(opts.SourceRole), URL: link, Title: title}
		if dateNode := node.Find(opts.DateSelector).First(); dateNode.Length() > 0 {
			if value, ok := dateNode.Attr("datetime"); ok {
				item.PublishedAt = parseOptionalTime(value)
			}
			if item.PublishedAt == nil {
				item.PublishedAt = parseOptionalTime(dateNode.Text())
			}
		}
		seen[link] = true
		out = append(out, item)
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("parse html %q: no article links found", sourceName)
	}
	return out, nil
}
