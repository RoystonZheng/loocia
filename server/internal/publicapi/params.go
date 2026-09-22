package publicapi

import (
	"errors"
	"net/url"
	"strconv"
	"time"

	"aihot-server/internal/items"
)

// errBadRequest signals a 400 to the handler.
var errBadRequest = errors.New("bad request")

const (
	defaultTake = 50
	maxTake     = 100
	sinceWindow = 7 * 24 * time.Hour
	futureSkew  = time.Minute
	maxQ        = 200
)

var apiCategories = map[string]bool{
	items.CategoryAIModels:   true,
	items.CategoryAIProducts: true,
	items.CategoryIndustry:   true,
	items.CategoryPaper:      true,
	items.CategoryTip:        true,
}

var apiSourceKinds = map[string]bool{
	"rss":   true,
	"html":  true,
	"mp":    true,
	"aihot": true,
}

// parseListParams validates the query against the openapi contract and returns
// the store-level ListParams. `now` is injected for testability. Returns
// errBadRequest on any contract violation (→ 400).
func parseListParams(q url.Values, now time.Time) (items.ListParams, error) {
	var p items.ListParams

	switch q.Get("mode") {
	case "", "selected":
		tru := true
		p.Selected = &tru
	case "all":
		// leave nil
	default:
		return p, errBadRequest
	}

	categories, err := parseMultiValue(q, "category", apiCategories)
	if err != nil {
		return p, err
	}
	p.Categories = categories

	// source_kind filter: rss/html/mp/aihot. Any other value → 400.
	sourceKinds, err := parseMultiValue(q, "source_kind", apiSourceKinds)
	if err != nil {
		return p, err
	}
	p.SourceKinds = sourceKinds

	if score := q.Get("score_min"); score != "" {
		n, err := strconv.Atoi(score)
		if err != nil || n < 1 || n > 5 {
			return p, errBadRequest
		}
		p.ScoreMin = &n
	}

	p.Limit = defaultTake
	if t := q.Get("take"); t != "" {
		n, err := strconv.Atoi(t)
		if err != nil || n < 1 || n > maxTake {
			return p, errBadRequest
		}
		p.Limit = n
	}

	// A text search queries the whole archive (公众号 backfill runs to years of
	// history); the default 7-day freshness floor applies only when browsing.
	qs := q.Get("q")
	isSearch := len([]rune(qs)) >= 2
	if isSearch {
		if len([]rune(qs)) > maxQ {
			qs = string([]rune(qs)[:maxQ])
		}
		p.Q = &qs
	}

	// The 公众号 (mp) corpus is historical, so browsing it must ignore the 7-day
	// freshness floor — otherwise the archive is invisible. Searching already
	// spans all time; explicitly viewing the mp source opens the archive too.
	archive := slicesContains(p.SourceKinds, "mp")
	allTime := isSearch || archive

	lower := now.Add(-sinceWindow)
	if s := q.Get("since"); s != "" {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return p, errBadRequest
		}
		if ts.After(now.Add(futureSkew)) {
			return p, errBadRequest
		}
		if !allTime && ts.Before(lower) {
			ts = lower // clamp to the window only when browsing fresh sources
		}
		tt := ts.UTC()
		p.Since = &tt
	} else if !allTime {
		p.Since = &lower // default window; search / mp archive → all-time
	}

	if cur := q.Get("cursor"); cur != "" {
		p.After = decodeCursor(cur)
	}

	return p, nil
}

func parseMultiValue(q url.Values, key string, allowed map[string]bool) ([]string, error) {
	var values []string
	seen := make(map[string]struct{})
	for _, value := range q[key] {
		if value == "" {
			continue
		}
		if !allowed[value] {
			return nil, errBadRequest
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values, nil
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
