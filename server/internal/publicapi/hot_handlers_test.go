package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aihot-server/internal/cluster"
)

type fakeHotStore struct {
	rows []cluster.HotTopicRow
	err  error
}

func (f *fakeHotStore) ListHotTopics(ctx context.Context) ([]cluster.HotTopicRow, error) {
	return f.rows, f.err
}

func TestHotTopicsEnvelopeAndShape(t *testing.T) {
	latest := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	f := &fakeHotStore{rows: []cluster.HotTopicRow{{
		ID: "h1", Title: "热点标题", URL: "https://x/h1", Permalink: "/items/h1",
		Source: "OpenAI Blog", SourceCount: 4, SourceNames: []string{"OpenAI Blog", "机器之心"},
		LatestAt: latest,
	}}}
	h := NewHotTopicsHandler(f)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var env struct {
		Count int              `json:"count"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 1 || len(env.Items) != 1 {
		t.Fatalf("envelope: %+v", env)
	}
	it := env.Items[0]
	for _, k := range []string{"id", "title", "url", "permalink", "source", "sourceCount", "sourceNames", "latestAt"} {
		if _, ok := it[k]; !ok {
			t.Fatalf("missing key %s: %v", k, it)
		}
	}
	// Internal fields must NOT leak.
	for _, banned := range []string{"heat", "clusterId", "cluster_id", "primaryItemId"} {
		if _, ok := it[banned]; ok {
			t.Fatalf("internal field %s leaked: %v", banned, it)
		}
	}
	if it["sourceCount"].(float64) != 4 {
		t.Fatalf("sourceCount: %v", it["sourceCount"])
	}
}

func TestHotTopicsEmptyIsArray(t *testing.T) {
	h := NewHotTopicsHandler(&fakeHotStore{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil))
	body := rr.Body.String()
	if !contains(body, `"items":[]`) {
		t.Fatalf("empty items must be [] not null: %s", body)
	}
}

func TestHotTopicsETagAnd304(t *testing.T) {
	f := &fakeHotStore{rows: []cluster.HotTopicRow{{ID: "h1", Title: "t", URL: "u", Permalink: "/items/h1",
		Source: "S", SourceCount: 2, SourceNames: []string{"S"}, LatestAt: time.Now().UTC()}}}
	h := NewHotTopicsHandler(f)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil))
	etag := rr.Header().Get("ETag")
	if etag == "" || etag[:6] != `W/"hot` {
		t.Fatalf("weak hot ETag expected, got %q", etag)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil)
	req.Header.Set("If-None-Match", etag)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusNotModified || rr2.Body.Len() != 0 {
		t.Fatalf("want empty 304, got %d (%d bytes)", rr2.Code, rr2.Body.Len())
	}
}

func TestHotTopicsStoreErrorIs500(t *testing.T) {
	h := NewHotTopicsHandler(&fakeHotStore{err: context.DeadlineExceeded})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rr.Code)
	}
}
