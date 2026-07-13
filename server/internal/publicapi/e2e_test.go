package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
	"aihot-server/internal/items"
)

func liveStore(t *testing.T) *items.Store {
	t.Helper()
	dsn := os.Getenv("AIHOT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIHOT_TEST_DATABASE_URL not set")
	}
	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	t.Cleanup(pool.Close)
	s := items.New(pool)
	if err := s.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE items"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func TestItemsEndpointLivePaginatesAndProjects(t *testing.T) {
	store := liveStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i := 0; i < 3; i++ {
		pub := now.Add(-time.Duration(i+1) * time.Hour)
		en := "EN"
		summary := "中文摘要"
		cat := items.CategoryAIModels
		score := 5 - i
		if err := store.Upsert(ctx, items.Item{
			ID: string(rune('a' + i)), Title: "标题", TitleEN: &en,
			URL: "https://x/" + string(rune('a'+i)), Permalink: "/items/" + string(rune('a'+i)),
			Source: "Src", PublishedAt: &pub, Summary: &summary, Category: &cat, Score: &score,
			AIRelevance: intp(5), AISelected: boolp(true), Selected: true, Present: true,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	h := NewItemsHandler(store, time.Now)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/items?take=2", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("page1 code: %d body=%s", rr.Code, rr.Body.String())
	}
	var p1 ItemList
	if err := json.Unmarshal(rr.Body.Bytes(), &p1); err != nil {
		t.Fatalf("decode p1: %v", err)
	}
	if p1.Count != 2 || !p1.HasNext || p1.NextCursor == nil {
		t.Fatalf("page1: %+v", p1)
	}
	if contains(rr.Body.String(), "ai_relevance") || contains(rr.Body.String(), "present") {
		t.Fatalf("internal field leaked: %s", rr.Body.String())
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/public/items?take=2&cursor="+*p1.NextCursor, nil))
	var p2 ItemList
	if err := json.Unmarshal(rr2.Body.Bytes(), &p2); err != nil {
		t.Fatalf("decode p2: %v", err)
	}
	if p2.Count != 1 || p2.HasNext {
		t.Fatalf("page2: %+v", p2)
	}
	seen := map[string]bool{}
	for _, it := range p1.Items {
		seen[it.ID] = true
	}
	for _, it := range p2.Items {
		if seen[it.ID] {
			t.Fatalf("id %s appeared on both pages", it.ID)
		}
	}
}

func intp(i int) *int    { return &i }
func boolp(b bool) *bool { return &b }
