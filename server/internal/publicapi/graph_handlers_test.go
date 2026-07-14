package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aihot-server/internal/terms"
)

type fakeGraphStore struct {
	cloud     []terms.CloudTerm
	neighbors []terms.Neighbor
	items     []terms.TermItem
	kind      string
	count     int
	err       error

	gotSince *time.Time // captured from the last call
	gotTerm  string
}

func (f *fakeGraphStore) Cloud(ctx context.Context, since *time.Time, limit int) ([]terms.CloudTerm, error) {
	f.gotSince = since
	return f.cloud, f.err
}
func (f *fakeGraphStore) Neighbors(ctx context.Context, term string, since *time.Time, limit int) ([]terms.Neighbor, error) {
	f.gotTerm, f.gotSince = term, since
	return f.neighbors, f.err
}
func (f *fakeGraphStore) ItemsForTerm(ctx context.Context, term string, since *time.Time, limit int) ([]terms.TermItem, error) {
	return f.items, f.err
}
func (f *fakeGraphStore) TermInfo(ctx context.Context, term string, since *time.Time) (string, int, error) {
	return f.kind, f.count, f.err
}

var fixedNow = func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) }

func TestGraphCloudShapeAndWindow(t *testing.T) {
	f := &fakeGraphStore{cloud: []terms.CloudTerm{{Term: "OpenAI", Kind: "entity", Count: 42}}}
	h := NewGraphCloudHandler(f, fixedNow)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/cloud", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var env struct {
		Terms []map[string]any `json:"terms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Terms) != 1 || env.Terms[0]["term"] != "OpenAI" || env.Terms[0]["kind"] != "entity" || env.Terms[0]["count"].(float64) != 42 {
		t.Fatalf("terms: %+v", env.Terms)
	}
	// 默认窗口 = 7d
	want := fixedNow().Add(-7 * 24 * time.Hour)
	if f.gotSince == nil || !f.gotSince.Equal(want) {
		t.Fatalf("default since: %v", f.gotSince)
	}
}

func TestGraphCloudWindowParams(t *testing.T) {
	cases := []struct {
		q      string
		isNil  bool
		offset time.Duration
	}{
		{"window=30d", false, 30 * 24 * time.Hour},
		{"window=all", true, 0},
		{"window=bogus", false, 7 * 24 * time.Hour}, // 非法值宽容回默认
	}
	for _, c := range cases {
		f := &fakeGraphStore{}
		h := NewGraphCloudHandler(f, fixedNow)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/cloud?"+c.q, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s code: %d", c.q, rr.Code)
		}
		if c.isNil {
			if f.gotSince != nil {
				t.Fatalf("%s: since should be nil, got %v", c.q, f.gotSince)
			}
		} else if f.gotSince == nil || !f.gotSince.Equal(fixedNow().Add(-c.offset)) {
			t.Fatalf("%s: since %v", c.q, f.gotSince)
		}
	}
}

func TestGraphCloudEmptyIsEmptyArray(t *testing.T) {
	h := NewGraphCloudHandler(&fakeGraphStore{}, fixedNow)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/cloud", nil))
	if rr.Body.String() != `{"terms":[]}` {
		t.Fatalf("body: %s", rr.Body.String())
	}
}

func TestGraphTermShapeAndEscaping(t *testing.T) {
	pub := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	f := &fakeGraphStore{
		kind: "entity", count: 42,
		neighbors: []terms.Neighbor{{Term: "推理模型", Kind: "topic", Weight: 18}},
		items:     []terms.TermItem{{ID: "i1", Title: "标题", URL: "https://x/i1", Permalink: "/items/i1", Source: "S", PublishedAt: &pub, Selected: true}},
	}
	h := NewGraphTermHandler(f, fixedNow)
	rr := httptest.NewRecorder()
	// URL 编码的中文词必须解回来
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/term/%E6%8E%A8%E7%90%86?window=30d", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	if f.gotTerm != "推理" {
		t.Fatalf("term: %q", f.gotTerm)
	}
	var env struct {
		Term      string           `json:"term"`
		Kind      string           `json:"kind"`
		Count     int              `json:"count"`
		Neighbors []map[string]any `json:"neighbors"`
		Items     []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Term != "推理" || env.Kind != "entity" || env.Count != 42 {
		t.Fatalf("head: %+v", env)
	}
	if len(env.Neighbors) != 1 || env.Neighbors[0]["weight"].(float64) != 18 {
		t.Fatalf("neighbors: %+v", env.Neighbors)
	}
	if len(env.Items) != 1 || env.Items[0]["permalink"] != "/items/i1" {
		t.Fatalf("items: %+v", env.Items)
	}
}

func TestGraphTermPercentIsNotDoubleDecoded(t *testing.T) {
	// httptest.NewRequest parses the target with url.Parse, so r.URL.Path
	// arrives already percent-decoded ("100%") — same as a production request
	// for /term/100%25. The handler must not unescape a second time.
	f := &fakeGraphStore{}
	h := NewGraphTermHandler(f, fixedNow)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/public/graph/term/100%25", nil)
	if req.URL.Path != "/api/public/graph/term/100%" {
		t.Fatalf("test setup: path not pre-decoded: %q", req.URL.Path)
	}
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d body: %s", rr.Code, rr.Body.String())
	}
	if f.gotTerm != "100%" {
		t.Fatalf("term: %q", f.gotTerm)
	}
}

func TestGraphTermMissingIs404(t *testing.T) {
	h := NewGraphTermHandler(&fakeGraphStore{}, fixedNow)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/term/", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("code: %d", rr.Code)
	}
}

func TestGraphTermUnknownIs200Empty(t *testing.T) {
	h := NewGraphTermHandler(&fakeGraphStore{}, fixedNow)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/term/nobody", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var env struct {
		Count     int   `json:"count"`
		Neighbors []any `json:"neighbors"`
		Items     []any `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 0 || len(env.Neighbors) != 0 || len(env.Items) != 0 {
		t.Fatalf("env: %+v", env)
	}
}
