package publicapi

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aihot-server/internal/items"
)

// Lister is the store surface the handler needs (satisfied by *items.Store).
type Lister interface {
	List(ctx context.Context, p items.ListParams) ([]items.Item, error)
}

// Clock returns the current time (injected for testability).
type Clock func() time.Time

// ItemsHandler serves GET /api/public/items.
type ItemsHandler struct {
	store Lister
	now   Clock
}

func NewItemsHandler(store Lister, now Clock) *ItemsHandler {
	return &ItemsHandler{store: store, now: now}
}

func (h *ItemsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, err := parseListParams(r.URL.Query(), h.now())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid query parameters")
		return
	}

	take := p.Limit
	p.Limit = take + 1

	rows, err := h.store.List(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	hasNext := len(rows) > take
	if hasNext {
		rows = rows[:take]
	}

	env := ItemList{
		Count: len(rows),
		Items: make([]PublicItem, 0, len(rows)),
	}
	for _, it := range rows {
		env.Items = append(env.Items, toPublic(it))
	}
	env.HasNext = hasNext
	if hasNext && len(rows) > 0 {
		last := rows[len(rows)-1]
		tok := encodeCursor(&items.Cursor{SortKey: last.SortKey(), ID: last.ID})
		env.NextCursor = &tok
	}

	body, err := json.Marshal(env)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encoding error")
		return
	}

	etag := weakETag(body)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, s-maxage=300, stale-while-revalidate=300")
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func weakETag(body []byte) string {
	sum := sha1.Sum(body)
	return fmt.Sprintf(`W/"items-%x"`, sum[:8])
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
