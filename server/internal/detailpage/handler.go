package detailpage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"aihot-server/internal/items"
)

// ItemGetter is the store surface the page needs (satisfied by *items.Store).
type ItemGetter interface {
	GetByID(ctx context.Context, id string) (*items.Item, error)
}

// Translator re-translates a body and names its model (satisfied by pipeline.Translator).
type Translator interface {
	Translate(ctx context.Context, body string) (string, error)
	Model() string
}

// BodyCNUpdater persists a re-translation (satisfied by *items.Store).
type BodyCNUpdater interface {
	UpdateBodyCN(ctx context.Context, id, cn, model string) error
}

// Handler server-side-renders GET /items/{id} as an HTML page and, when
// retranslate is enabled, handles POST /items/{id}/retranslate.
type Handler struct {
	store ItemGetter
	tr    Translator    // optional; nil → retranslate returns 503
	upd   BodyCNUpdater // optional; nil → retranslate returns 503
}

func NewHandler(store ItemGetter) *Handler {
	return &Handler{store: store}
}

// WithRetranslate enables the POST /items/{id}/retranslate endpoint.
func (h *Handler) WithRetranslate(tr Translator, upd BodyCNUpdater) *Handler {
	h.tr, h.upd = tr, upd
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if id, ok := retranslateID(r.URL.Path); ok {
			h.retranslate(w, r, id)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

// retranslateID matches POST /items/{id}/retranslate and returns the id.
func retranslateID(path string) (string, bool) {
	rest := strings.TrimPrefix(path, "/items/")
	if rest == path {
		return "", false
	}
	id := strings.TrimSuffix(rest, "/retranslate")
	if id == rest || id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

func writeJSONOK(w http.ResponseWriter, ok bool, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"ok":%t}`, ok)
}

func (h *Handler) retranslate(w http.ResponseWriter, r *http.Request, id string) {
	if h.tr == nil || h.upd == nil {
		writeJSONOK(w, false, http.StatusServiceUnavailable)
		return
	}
	it, err := h.store.GetByID(r.Context(), id)
	if errors.Is(err, items.ErrNotFound) {
		writeJSONOK(w, false, http.StatusNotFound)
		return
	}
	if err != nil {
		writeJSONOK(w, false, http.StatusInternalServerError)
		return
	}
	if it.SourceKind != "rss" || it.Body == nil || strings.TrimSpace(*it.Body) == "" {
		writeJSONOK(w, false, http.StatusBadRequest)
		return
	}
	cn, err := h.tr.Translate(r.Context(), *it.Body)
	if err != nil || strings.TrimSpace(cn) == "" {
		writeJSONOK(w, false, http.StatusOK)
		return
	}
	if err := h.upd.UpdateBodyCN(r.Context(), id, cn, h.tr.Model()); err != nil {
		writeJSONOK(w, false, http.StatusInternalServerError)
		return
	}
	writeJSONOK(w, true, http.StatusOK)
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

// compile-time assertions that *items.Store satisfies the store surfaces.
var (
	_ ItemGetter    = (*items.Store)(nil)
	_ BodyCNUpdater = (*items.Store)(nil)
)
