package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// JSONAPIOptions describes the small amount of mapping needed for a JSON
// endpoint. Paths are dotted JSON paths, for example "data.items" or
// "results". Header values are environment variable names so credentials never
// need to be committed to a source plan.
type JSONAPIOptions struct {
	SourceKind      string
	SourceRole      string
	ItemsPath       string
	TitlePath       string
	URLPath         string
	ContentPath     string
	PublishedAtPath string
	SourcePath      string
	HeaderEnv       map[string]string
	MaxItemsPerRun  int
	Timeout         time.Duration
}

type JSONAPISource struct {
	name     string
	endpoint string
	options  JSONAPIOptions
	client   *http.Client
	baseURL  *url.URL
}

func NewJSONAPISource(name, endpoint string, opts JSONAPIOptions) *JSONAPISource {
	if opts.SourceKind == "" {
		opts.SourceKind = SourceKindAPI
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	base, _ := url.Parse(endpoint)
	return &JSONAPISource{
		name: name, endpoint: endpoint, options: opts,
		client: newSourceHTTPClient(opts.Timeout), baseURL: base,
	}
}

func (s *JSONAPISource) Name() string { return s.name }

func (s *JSONAPISource) Fetch(ctx context.Context) ([]RawItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "aihot-ingest/0.1")
	for header, envName := range s.options.HeaderEnv {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			req.Header.Set(header, value)
		}
	}
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
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, fmt.Errorf("parse JSON API %q: %w", s.name, err)
	}
	rows := jsonRows(document, s.options.ItemsPath)
	if len(rows) == 0 {
		return nil, fmt.Errorf("parse JSON API %q: no items found", s.name)
	}
	items := make([]RawItem, 0, len(rows))
	for _, row := range rows {
		item, ok := s.toRaw(row)
		if !ok {
			continue
		}
		items = append(items, item)
		if s.options.MaxItemsPerRun > 0 && len(items) >= s.options.MaxItemsPerRun {
			break
		}
	}
	return items, nil
}

func (s *JSONAPISource) toRaw(row any) (RawItem, bool) {
	if rowMap, ok := row.(map[string]any); ok {
		urlValue := firstString(rowMap, s.options.URLPath, "url", "link", "web_url", "href", "permalink")
		title := firstString(rowMap, s.options.TitlePath, "title", "name", "headline")
		if urlValue == "" || title == "" {
			return RawItem{}, false
		}
		link := resolveURL(s.baseURL, urlValue)
		item := RawItem{ID: RawID(link), Source: s.name, SourceKind: s.options.SourceKind, SourceRole: DefaultSourceRole(s.options.SourceRole), URL: link, Title: title}
		if content := firstString(rowMap, s.options.ContentPath, "summary", "description", "content", "body", "text"); content != "" {
			item.RawContent = &content
		}
		if source := firstString(rowMap, s.options.SourcePath, "source", "author", "publisher"); source != "" && item.Source == "" {
			item.Source = source
		}
		if published := firstValue(rowMap, s.options.PublishedAtPath, "published_at", "publishedAt", "published", "date", "created_at", "createdAt"); published != nil {
			item.PublishedAt = parseJSONTime(published)
		}
		return item, true
	}
	return RawItem{}, false
}

func jsonRows(document any, configuredPath string) []any {
	value := document
	if configuredPath != "" {
		value = lookupJSONPath(document, configuredPath)
	} else if object, ok := document.(map[string]any); ok {
		for _, key := range []string{"items", "results", "data", "entries", "articles"} {
			if candidate, exists := object[key]; exists {
				value = candidate
				break
			}
		}
	}
	rows, _ := value.([]any)
	return rows
}

func lookupJSONPath(value any, path string) any {
	for _, part := range strings.Split(strings.Trim(path, ". "), ".") {
		if part == "" {
			continue
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value, ok = object[part]
		if !ok {
			return nil
		}
	}
	return value
}

func firstValue(object map[string]any, configured string, defaults ...string) any {
	if configured != "" {
		if value := lookupJSONPath(object, configured); value != nil {
			return value
		}
	}
	for _, key := range defaults {
		if value, ok := object[key]; ok {
			return value
		}
	}
	return nil
}

func firstString(object map[string]any, configured string, defaults ...string) string {
	value := firstValue(object, configured, defaults...)
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

func parseJSONTime(value any) *time.Time {
	if number, ok := value.(float64); ok {
		if number > 1e12 {
			number /= 1000
		}
		t := time.Unix(int64(number), 0).UTC()
		return &t
	}
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return parseOptionalTime(text)
}

func resolveURL(base *url.URL, raw string) string {
	if base == nil {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(parsed).String()
}
