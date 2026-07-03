package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aihot-server/internal/items"
)

// fakeLister returns canned items and records the params it received.
type fakeLister struct {
	got    items.ListParams
	result []items.Item
	err    error
}

func (f *fakeLister) List(ctx context.Context, p items.ListParams) ([]items.Item, error) {
	f.got = p
	return f.result, f.err
}

func mkItem(id string, pub time.Time) items.Item {
	summary := "s-" + id
	cat := items.CategoryAIModels
	score := 70
	return items.Item{
		ID: id, Title: "t-" + id, URL: "https://x/" + id, Permalink: "/items/" + id,
		Source: "S", PublishedAt: &pub, Summary: &summary, Category: &cat, Score: &score,
		Selected: true, Present: true,
	}
}

var hNow = time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)

func newTestHandler(f *fakeLister) *ItemsHandler {
	return NewItemsHandler(f, func() time.Time { return hNow })
}

func TestHandlerReturnsEnvelope(t *testing.T) {
	base := hNow.Add(-3 * 24 * time.Hour)
	// take=2 → handler requests take+1 (3); returning 3 rows exercises the
	// "detect next page, trim to take" path so hasNext becomes true.
	f := &fakeLister{result: []items.Item{
		mkItem("a", base.Add(time.Hour)), mkItem("b", base), mkItem("c", base.Add(-time.Hour)),
	}}
	h := newTestHandler(f)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/items?take=2", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: %q", ct)
	}
	var env ItemList
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 2 || len(env.Items) != 2 {
		t.Fatalf("envelope: %+v", env)
	}
	if !env.HasNext || env.NextCursor == nil {
		t.Fatalf("expected hasNext + nextCursor: %+v", env)
	}
	if f.got.Limit != 3 {
		t.Fatalf("handler should request take+1 (3), got %d", f.got.Limit)
	}
}

func TestHandlerNoNextWhenUnderLimit(t *testing.T) {
	f := &fakeLister{result: []items.Item{mkItem("a", hNow.Add(-time.Hour))}}
	h := newTestHandler(f)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/items?take=10", nil))
	var env ItemList
	_ = json.Unmarshal(rr.Body.Bytes(), &env)
	if env.HasNext || env.NextCursor != nil {
		t.Fatalf("single page should have no next: %+v", env)
	}
	if env.Count != 1 {
		t.Fatalf("count: %d", env.Count)
	}
}

func TestHandlerBadParamIs400(t *testing.T) {
	h := newTestHandler(&fakeLister{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/items?take=999", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestHandlerETagAnd304(t *testing.T) {
	f := &fakeLister{result: []items.Item{mkItem("a", hNow.Add(-time.Hour))}}
	h := newTestHandler(f)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/items", nil))
	etag := rr.Header().Get("ETag")
	if etag == "" || etag[:4] != `W/"i` {
		t.Fatalf("weak items ETag expected, got %q", etag)
	}
	if cc := rr.Header().Get("Cache-Control"); cc == "" {
		t.Fatal("Cache-Control missing")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/public/items", nil)
	req.Header.Set("If-None-Match", etag)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusNotModified {
		t.Fatalf("want 304, got %d", rr2.Code)
	}
	if rr2.Body.Len() != 0 {
		t.Fatalf("304 body should be empty, got %d bytes", rr2.Body.Len())
	}
}

func TestHandlerListErrorIs500(t *testing.T) {
	f := &fakeLister{err: context.DeadlineExceeded}
	h := newTestHandler(f)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/items", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rr.Code)
	}
}
