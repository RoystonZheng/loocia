package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// SourceCandidate is evidence from a bounded source discovery pass. It is
// intentionally reviewable JSON rather than an opaque auto-generated config.
type SourceCandidate struct {
	URL          string   `json:"url"`
	Kind         string   `json:"kind"`
	Adapter      string   `json:"adapter"`
	Confidence   float64  `json:"confidence"`
	Evidence     []string `json:"evidence,omitempty"`
	RequiresAuth bool     `json:"requires_auth,omitempty"`
}

type DiscoveryResult struct {
	Seed       string            `json:"seed"`
	Request    string            `json:"request,omitempty"`
	Candidates []SourceCandidate `json:"candidates"`
	Warnings   []string          `json:"warnings,omitempty"`
}

// DiscoverSources checks one seed and a small set of conventional endpoints.
// It does not crawl a site or guess credentials. RSS/Atom links exposed by the
// page are preferred, followed by JSON endpoints and then a generic HTML list.
func DiscoverSources(ctx context.Context, seed, request string, timeout time.Duration, maxCandidates int) (DiscoveryResult, error) {
	result := DiscoveryResult{Seed: seed, Request: request}
	base, err := url.Parse(seed)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return result, fmt.Errorf("invalid seed URL %q", seed)
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := newSourceHTTPClient(timeout)
	seen := map[string]bool{}
	add := func(candidate SourceCandidate) {
		if candidate.URL == "" || seen[candidate.URL] || (maxCandidates > 0 && len(result.Candidates) >= maxCandidates) {
			return
		}
		seen[candidate.URL] = true
		result.Candidates = append(result.Candidates, candidate)
	}

	body, contentType, status, err := discoverHTTP(ctx, client, seed)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	} else if status >= 200 && status < 300 {
		if looksLikeFeed(body, contentType) {
			add(SourceCandidate{URL: seed, Kind: SourceKindRSS, Adapter: "rss", Confidence: 0.95, Evidence: []string{"seed response is RSS or Atom"}})
		} else if looksLikeJSON(body, contentType) {
			add(SourceCandidate{URL: seed, Kind: SourceKindAPI, Adapter: "json_api", Confidence: 0.9, Evidence: []string{"seed response is JSON"}})
		} else {
			for _, feed := range discoverFeedLinks(body, base) {
				add(SourceCandidate{URL: feed, Kind: SourceKindRSS, Adapter: "rss", Confidence: 0.9, Evidence: []string{"HTML alternate link advertises RSS/Atom"}})
			}
			add(SourceCandidate{URL: seed, Kind: SourceKindHTML, Adapter: "html_list", Confidence: 0.35, Evidence: []string{"seed is an HTML listing; selectors need verification"}})
		}
	}

	for _, suffix := range []string{"/feed", "/feed.xml", "/rss.xml", "/atom.xml", "/rss", "/api", "/api/feed", "/openapi.json"} {
		if maxCandidates > 0 && len(result.Candidates) >= maxCandidates {
			break
		}
		candidateURL := strings.TrimRight(seed, "/") + suffix
		candidateBody, candidateType, candidateStatus, probeErr := discoverHTTP(ctx, client, candidateURL)
		if probeErr != nil || candidateStatus < 200 || candidateStatus >= 300 {
			continue
		}
		if looksLikeFeed(candidateBody, candidateType) {
			add(SourceCandidate{URL: candidateURL, Kind: SourceKindRSS, Adapter: "rss", Confidence: 0.75, Evidence: []string{"conventional feed endpoint responded with RSS/Atom"}})
		} else if looksLikeJSON(candidateBody, candidateType) {
			add(SourceCandidate{URL: candidateURL, Kind: SourceKindAPI, Adapter: "json_api", Confidence: 0.7, Evidence: []string{"conventional endpoint responded with JSON"}})
		}
	}
	if len(result.Candidates) == 0 && len(result.Warnings) == 0 {
		result.Warnings = append(result.Warnings, "no feed, API, or HTML candidate was confirmed")
	}
	return result, nil
}

func discoverHTTP(ctx context.Context, client *http.Client, target string) ([]byte, string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.Header.Set("Accept", "text/html, application/atom+xml, application/rss+xml, application/json;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", "aihot-source-discovery/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return body, strings.ToLower(resp.Header.Get("Content-Type")), resp.StatusCode, readErr
}

func discoverFeedLinks(data []byte, base *url.URL) []string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	doc.Find("link[rel]").Each(func(_ int, node *goquery.Selection) {
		rel, _ := node.Attr("rel")
		typeValue, _ := node.Attr("type")
		href, _ := node.Attr("href")
		if !strings.Contains(strings.ToLower(rel), "alternate") || !isFeedType(typeValue) || strings.TrimSpace(href) == "" {
			return
		}
		parsed, err := base.Parse(strings.TrimSpace(href))
		if err != nil || parsed.Host != base.Host {
			return
		}
		link := parsed.String()
		if !seen[link] {
			seen[link] = true
			out = append(out, link)
		}
	})
	return out
}

func isFeedType(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "rss") || strings.Contains(value, "atom") || strings.Contains(value, "feed+json")
}

func looksLikeFeed(data []byte, contentType string) bool {
	contentType = strings.ToLower(contentType)
	trimmed := strings.TrimSpace(string(data))
	return strings.Contains(contentType, "rss") || strings.Contains(contentType, "atom") || strings.HasPrefix(trimmed, "<?xml") && (strings.Contains(trimmed, "<rss") || strings.Contains(trimmed, "<feed"))
}

func looksLikeJSON(data []byte, contentType string) bool {
	if strings.Contains(strings.ToLower(contentType), "json") {
		return json.Valid(data)
	}
	return json.Valid(data) && (strings.HasPrefix(strings.TrimSpace(string(data)), "{") || strings.HasPrefix(strings.TrimSpace(string(data)), "["))
}
