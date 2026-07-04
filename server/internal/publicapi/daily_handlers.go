package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aihot-server/internal/daily"
)

// DailyStore is the read surface the daily handlers need (satisfied by *daily.Store).
type DailyStore interface {
	GetLatest(ctx context.Context) (*daily.Report, error)
	GetByDate(ctx context.Context, date string) (*daily.Report, error)
	ListRecent(ctx context.Context, take int) ([]daily.Entry, error)
}

func writeDailyJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// normalizeReport replaces nil Sections/Flashes with empty slices so the JSON
// body carries [] (the openapi contract's arrays), never null.
func normalizeReport(rep *daily.Report) {
	if rep.Sections == nil {
		rep.Sections = []daily.Section{}
	}
	if rep.Flashes == nil {
		rep.Flashes = []daily.Flash{}
	}
}

// NewLatestDailyHandler serves GET /api/public/daily.
func NewLatestDailyHandler(s DailyStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rep, err := s.GetLatest(r.Context())
		if errors.Is(err, daily.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no daily report available")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		normalizeReport(rep)
		writeDailyJSON(w, rep)
	})
}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// NewDailyByDateHandler serves GET /api/public/daily/{date} (mounted at the
// "/api/public/daily/" subtree; the date is the path suffix).
func NewDailyByDateHandler(s DailyStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := strings.TrimPrefix(r.URL.Path, "/api/public/daily/")
		if !datePattern.MatchString(date) {
			writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
			return
		}
		// The regex only checks shape; reject non-calendar dates (e.g. 1999-13-99).
		if _, err := time.Parse("2006-01-02", date); err != nil {
			writeError(w, http.StatusBadRequest, "date must be a valid YYYY-MM-DD calendar date")
			return
		}
		rep, err := s.GetByDate(r.Context(), date)
		if errors.Is(err, daily.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no daily report for that date")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		normalizeReport(rep)
		writeDailyJSON(w, rep)
	})
}

// dailiesEnvelope is the openapi DailyEntries shape.
type dailiesEnvelope struct {
	Count int           `json:"count"`
	Items []daily.Entry `json:"items"`
}

// NewDailiesHandler serves GET /api/public/dailies (take: strict int 1-180, default 30).
func NewDailiesHandler(s DailyStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		take := 30
		if t := r.URL.Query().Get("take"); t != "" {
			n, err := strconv.Atoi(t)
			if err != nil || n < 1 || n > 180 {
				writeError(w, http.StatusBadRequest, "take must be an integer 1-180")
				return
			}
			take = n
		}
		entries, err := s.ListRecent(r.Context(), take)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if entries == nil {
			entries = []daily.Entry{}
		}
		writeDailyJSON(w, dailiesEnvelope{Count: len(entries), Items: entries})
	})
}
