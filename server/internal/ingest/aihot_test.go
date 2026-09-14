package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAIHOTSourceMapsOriginalAndSecondaryItems(t *testing.T) {
	resp := aihotItemsResponse{Items: []aihotItem{
		{
			ID:      "a1",
			Title:   "AIHOT 中文标题",
			Summary: strPtr("AIHOT summary"),
			Links: struct {
				AIHOT    string `json:"aihot"`
				Original string `json:"original"`
			}{AIHOT: "https://aihot.news/items/a1", Original: "https://source.example/a1"},
			PublishedAt: strPtr("2026-09-12T08:30:00Z"),
		},
		{
			ID:    "b1",
			Title: "Secondary one",
			Links: struct {
				AIHOT    string `json:"aihot"`
				Original string `json:"original"`
			}{AIHOT: "https://aihot.news/items/b1"},
		},
		{
			ID:    "b2",
			Title: "Secondary two",
			Links: struct {
				AIHOT    string `json:"aihot"`
				Original string `json:"original"`
			}{AIHOT: "https://aihot.news/items/b2"},
		},
	}}
	resp.Items[0].Source.Name = "Original Source"

	src := NewAIHOTSource("AIHOT", "https://aihot.news/api/v1/items", AIHOTSourceOptions{
		SourceRole:         SourceRoleDiscovery,
		MaxItemsPerRun:     50,
		MaxSecondaryPerRun: 1,
	})
	items := src.toRawItems(resp.Items)
	if len(items) != 2 {
		t.Fatalf("want original plus one capped secondary, got %d", len(items))
	}
	if items[0].URL != "https://source.example/a1" || items[0].ID != RawID("https://source.example/a1") {
		t.Fatalf("original mapping: %+v", items[0])
	}
	if items[0].Source != "Original Source" || items[0].SourceKind != SourceKindAIHOT || items[0].SourceRole != SourceRoleDiscovery {
		t.Fatalf("source fields: %+v", items[0])
	}
	if items[0].RawContent == nil || *items[0].RawContent != "AIHOT summary" {
		t.Fatalf("summary: %v", items[0].RawContent)
	}
	if items[1].Source != "AIHOT · 二手线索" || items[1].ID != RawID("aihot:b1") {
		t.Fatalf("secondary mapping: %+v", items[1])
	}
}

func TestAIHOTSourceFetchBuildsQueryAndParses(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		if ua := r.Header.Get("User-Agent"); ua != "aihot-ingest/0.1" {
			t.Fatalf("user-agent: %q", ua)
		}
		payload := map[string]any{
			"items": []map[string]any{{
				"id": "a1", "title": "Title",
				"source": map[string]any{"name": "Source"},
				"links":  map[string]any{"aihot": "https://aihot.news/items/a1", "original": "https://source.example/a1"},
			}},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	src := NewAIHOTSource("AIHOT", srv.URL, AIHOTSourceOptions{MaxItemsPerRun: 17, MaxSecondaryPerRun: 10})
	items, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if gotQuery.Get("mode") != "all" || gotQuery.Get("window") != "7d" || gotQuery.Get("by") != "published" || gotQuery.Get("limit") != "17" {
		t.Fatalf("query: %s", gotQuery.Encode())
	}
}

func TestAIHOTSourceFetchStatusHandling(t *testing.T) {
	t.Run("not modified", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotModified)
		}))
		defer srv.Close()
		items, err := NewAIHOTSource("AIHOT", srv.URL, AIHOTSourceOptions{}).Fetch(context.Background())
		if err != nil {
			t.Fatalf("304 should not error: %v", err)
		}
		if len(items) != 0 {
			t.Fatalf("304 should return no items, got %d", len(items))
		}
	})
	t.Run("retry after", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "limited", http.StatusTooManyRequests)
		}))
		defer srv.Close()
		_, err := NewAIHOTSource("AIHOT", srv.URL, AIHOTSourceOptions{}).Fetch(context.Background())
		if err == nil || !strings.Contains(err.Error(), "retry-after 60") {
			t.Fatalf("expected retry-after error, got %v", err)
		}
	})
}

func strPtr(s string) *string { return &s }
