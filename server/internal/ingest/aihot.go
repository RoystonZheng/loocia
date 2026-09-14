package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type AIHOTSourceOptions struct {
	SourceRole         string
	MaxItemsPerRun     int
	MaxSecondaryPerRun int
	Window             string
	Timeout            time.Duration
}

type AIHOTSource struct {
	name         string
	endpoint     string
	sourceRole   string
	maxItems     int
	maxSecondary int
	window       string
	client       *http.Client
}

func NewAIHOTSource(name, endpoint string, opts AIHOTSourceOptions) *AIHOTSource {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxItems := opts.MaxItemsPerRun
	if maxItems <= 0 {
		maxItems = 50
	}
	maxSecondary := opts.MaxSecondaryPerRun
	if maxSecondary <= 0 {
		maxSecondary = 10
	}
	window := opts.Window
	if window == "" {
		window = "7d"
	}
	return &AIHOTSource{
		name:         name,
		endpoint:     endpoint,
		sourceRole:   DefaultSourceRole(opts.SourceRole),
		maxItems:     maxItems,
		maxSecondary: maxSecondary,
		window:       window,
		client:       &http.Client{Timeout: timeout},
	}
}

func (s *AIHOTSource) Name() string { return s.name }

func (s *AIHOTSource) Fetch(ctx context.Context) ([]RawItem, error) {
	endpoint, err := s.requestURL()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "aihot-ingest/0.1")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", s.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, nil
	}
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
	var page aihotItemsResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("parse %q json: %w", s.name, err)
	}
	return s.toRawItems(page.Items), nil
}

func (s *AIHOTSource) requestURL() (string, error) {
	u, err := url.Parse(s.endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if q.Get("mode") == "" {
		q.Set("mode", "all")
	}
	if q.Get("window") == "" {
		q.Set("window", s.window)
	}
	if q.Get("by") == "" {
		q.Set("by", "published")
	}
	if q.Get("limit") == "" {
		q.Set("limit", strconv.Itoa(s.maxItems))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type aihotItemsResponse struct {
	Items []aihotItem `json:"items"`
}

type aihotItem struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle *string `json:"originalTitle"`
	Summary       *string `json:"summary"`
	Source        struct {
		Name string `json:"name"`
	} `json:"source"`
	Links struct {
		AIHOT    string `json:"aihot"`
		Original string `json:"original"`
	} `json:"links"`
	PublishedAt *string `json:"publishedAt"`
}

func (s *AIHOTSource) toRawItems(items []aihotItem) []RawItem {
	var out []RawItem
	secondary := 0
	for _, item := range items {
		if s.maxItems > 0 && len(out) >= s.maxItems {
			break
		}
		title := strings.TrimSpace(item.Title)
		if title == "" && item.OriginalTitle != nil {
			title = strings.TrimSpace(*item.OriginalTitle)
		}
		if title == "" {
			continue
		}
		rawURL := strings.TrimSpace(item.Links.Original)
		source := strings.TrimSpace(item.Source.Name)
		idSeed := rawURL
		if rawURL == "" {
			if secondary >= s.maxSecondary {
				continue
			}
			rawURL = strings.TrimSpace(item.Links.AIHOT)
			if rawURL == "" {
				continue
			}
			source = "AIHOT · 二手线索"
			if item.ID != "" {
				idSeed = "aihot:" + item.ID
			} else {
				idSeed = rawURL
			}
			secondary++
		} else if source == "" {
			source = s.name
		}
		r := RawItem{
			ID:         RawID(idSeed),
			Source:     source,
			SourceKind: SourceKindAIHOT,
			SourceRole: DefaultSourceRole(s.sourceRole),
			URL:        rawURL,
			Title:      title,
		}
		if item.Summary != nil && strings.TrimSpace(*item.Summary) != "" {
			summary := strings.TrimSpace(*item.Summary)
			r.RawContent = &summary
		}
		if item.PublishedAt != nil {
			if published := parseOptionalTime(*item.PublishedAt); published != nil {
				r.PublishedAt = published
			}
		}
		out = append(out, r)
	}
	return out
}
