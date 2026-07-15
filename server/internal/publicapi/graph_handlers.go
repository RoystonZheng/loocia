package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aihot-server/internal/terms"
)

// GraphStore is the read surface the graph handlers need (satisfied by *terms.Store).
type GraphStore interface {
	Cloud(ctx context.Context, since, until *time.Time, limit int) ([]terms.CloudTerm, error)
	Neighbors(ctx context.Context, term string, since, until *time.Time, limit int) ([]terms.Neighbor, error)
	ItemsForTerm(ctx context.Context, term string, since, until *time.Time, limit int) ([]terms.TermItem, error)
	TermInfo(ctx context.Context, term string, since, until *time.Time) (kind string, count int, err error)
}

// beijing is the calendar day used for the per-day (?date=) cloud, matching the
// daily report's Beijing-midnight window.
var beijing = time.FixedZone("CST", 8*3600)

// parseGraphRange resolves the time filter. An explicit ?date=YYYY-MM-DD (a
// Beijing calendar day) → [dayStart, nextDay) so each day gets its own cloud;
// otherwise ?window=7d|30d|all → (since, nil] as before.
func parseGraphRange(q url.Values, now time.Time) (since, until *time.Time) {
	if d := q.Get("date"); d != "" {
		if day, err := time.ParseInLocation("2006-01-02", d, beijing); err == nil {
			s := day
			u := day.AddDate(0, 0, 1)
			return &s, &u
		}
	}
	return parseWindow(q.Get("window"), now), nil
}

const (
	cloudLimit     = 80
	neighborLimit  = 20
	termItemsLimit = 20
)

// parseWindow maps ?window= to a since-cutoff: 7d (default, also the fallback
// for garbage) / 30d / all→nil. Lenient on purpose — public API style.
func parseWindow(v string, now time.Time) *time.Time {
	switch v {
	case "all":
		return nil
	case "30d":
		t := now.Add(-30 * 24 * time.Hour)
		return &t
	default: // "", "7d", anything else
		t := now.Add(-7 * 24 * time.Hour)
		return &t
	}
}

type cloudTermJSON struct {
	Term  string `json:"term"`
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type cloudResponse struct {
	Terms []cloudTermJSON `json:"terms"`
}

// NewGraphCloudHandler serves GET /api/public/graph/cloud?window=7d|30d|all.
func NewGraphCloudHandler(s GraphStore, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		since, until := parseGraphRange(r.URL.Query(), now())
		rows, err := s.Cloud(r.Context(), since, until, cloudLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		env := cloudResponse{Terms: make([]cloudTermJSON, 0, len(rows))}
		for _, c := range rows {
			env.Terms = append(env.Terms, cloudTermJSON{Term: c.Term, Kind: c.Kind, Count: c.Count})
		}
		writeJSON(w, env)
	})
}

type neighborJSON struct {
	Term   string `json:"term"`
	Kind   string `json:"kind"`
	Weight int    `json:"weight"`
}

type termResponse struct {
	Term      string         `json:"term"`
	Kind      string         `json:"kind"`
	Count     int            `json:"count"`
	Neighbors []neighborJSON `json:"neighbors"`
	Items     []PublicItem   `json:"items"`
}

// NewGraphTermHandler serves GET /api/public/graph/term/{term}?window=….
// Unknown terms answer 200 with empty arrays (easier for the frontend); only a
// missing path segment is a 404.
func NewGraphTermHandler(s GraphStore, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.URL.Path is already percent-decoded by net/http — no extra
		// unescape, or terms containing '%' would double-decode/404.
		term := strings.TrimPrefix(r.URL.Path, "/api/public/graph/term/")
		if term == "" {
			writeError(w, http.StatusNotFound, "term not found")
			return
		}
		since, until := parseGraphRange(r.URL.Query(), now())

		kind, count, err := s.TermInfo(r.Context(), term, since, until)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		ns, err := s.Neighbors(r.Context(), term, since, until, neighborLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		its, err := s.ItemsForTerm(r.Context(), term, since, until, termItemsLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		env := termResponse{Term: term, Kind: kind, Count: count,
			Neighbors: make([]neighborJSON, 0, len(ns)), Items: make([]PublicItem, 0, len(its))}
		for _, n := range ns {
			env.Neighbors = append(env.Neighbors, neighborJSON{Term: n.Term, Kind: n.Kind, Weight: n.Weight})
		}
		for _, it := range its {
			env.Items = append(env.Items, PublicItem{
				ID: it.ID, Title: it.Title, TitleEN: it.TitleEN, URL: it.URL,
				Permalink: it.Permalink, Source: it.Source, PublishedAt: it.PublishedAt,
				Summary: it.Summary, ImageURL: it.ImageURL, Category: it.Category,
				Score: it.Score, Selected: it.Selected,
			})
		}
		writeJSON(w, env)
	})
}

// writeJSON marshals and writes a 200 JSON body (500 on marshal failure).
// Deliberately no ETag (unlike hot_handlers): responses vary per window/term
// query and the 5-min CDN cache covers it.
func writeJSON(w http.ResponseWriter, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encoding error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, s-maxage=300, stale-while-revalidate=300")
	_, _ = w.Write(body)
}
