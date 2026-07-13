package detailpage

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"aihot-server/internal/items"
)

// ItemGetter is the store surface the page needs (satisfied by *items.Store).
type ItemGetter interface {
	GetByID(ctx context.Context, id string) (*items.Item, error)
}

// Handler server-side-renders GET /items/{id} as an HTML page.
type Handler struct {
	store ItemGetter
}

func NewHandler(store ItemGetter) *Handler {
	return &Handler{store: store}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/items/")
	if id == "" || strings.Contains(id, "/") {
		h.renderNotFound(w)
		return
	}

	it, err := h.store.GetByID(r.Context(), id)
	if errors.Is(err, items.ErrNotFound) {
		h.renderNotFound(w)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Visibility gate: withdrawn (not present) or a "完全重复" duplicate → 404.
	if !it.Present || it.DuplicateOfID != nil {
		h.renderNotFound(w)
		return
	}

	if r.URL.Query().Get("format") == "md" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("X-Robots-Tag", "noindex")
		w.Header().Set("Content-Disposition", `attachment; filename="aicool-`+shortID(id)+`.md"`)
		_, _ = w.Write([]byte(buildMarkdown(it)))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	if err := pageTemplate.Execute(w, toViewModel(it)); err != nil {
		// header already sent on partial write; best-effort log-free fallback
		return
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func (h *Handler) renderNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(http.StatusNotFound)
	_ = notFoundTemplate.Execute(w, nil)
}

// compile-time assertion that *items.Store satisfies ItemGetter
var _ ItemGetter = (*items.Store)(nil)
