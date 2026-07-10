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

	if c := q.Get("category"); c != "" {
		if !apiCategories[c] {
			return p, errBadRequest
		}
		cc := c
		p.Category = &cc
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

	lower := now.Add(-sinceWindow)
	if s := q.Get("since"); s != "" {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return p, errBadRequest
		}
		if ts.After(now.Add(futureSkew)) {
			return p, errBadRequest
		}
		if !isSearch && ts.Before(lower) {
			ts = lower // clamp to the window only when browsing
		}
		tt := ts.UTC()
		p.Since = &tt
	} else if !isSearch {
		p.Since = &lower // default window; a search omits it → all-time
	}

	if cur := q.Get("cursor"); cur != "" {
		p.After = decodeCursor(cur)
	}

	return p, nil
}
