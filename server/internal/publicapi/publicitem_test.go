package publicapi

import (
	"encoding/json"
	"testing"
	"time"

	"aihot-server/internal/items"
)

func TestToPublicExposesOnlyPublicFields(t *testing.T) {
	pub := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	en := "English"
	summary := "摘要"
	cat := items.CategoryAIModels
	score := 88
	rel := 91
	sel := true
	it := items.Item{
		ID: "id1", Title: "标题", TitleEN: &en, URL: "https://x/y", Permalink: "/items/id1",
		Source: "Src", PublishedAt: &pub, Summary: &summary, Category: &cat, Score: &score,
		AIRelevance: &rel, AISelected: &sel, Selected: true, Present: true,
	}

	p := toPublic(it)
	if p.ID != "id1" || p.Title != "标题" || p.URL != "https://x/y" || p.Permalink != "/items/id1" {
		t.Fatalf("basic fields: %+v", p)
	}
	if p.TitleEN == nil || *p.TitleEN != "English" || p.Summary == nil || *p.Summary != "摘要" {
		t.Fatalf("nullable fields: %+v", p)
	}
	if p.Category == nil || *p.Category != "ai-models" || p.Score == nil || *p.Score != 88 || !p.Selected {
		t.Fatalf("category/score/selected: %+v", p)
	}
	if p.PublishedAt == nil || !p.PublishedAt.Equal(pub) {
		t.Fatalf("publishedAt: %v", p.PublishedAt)
	}

	b, _ := json.Marshal(p)
	js := string(b)
	for _, banned := range []string{"ai_relevance", "aiRelevance", "ai_selected", "aiSelected",
		"cluster", "duplicate", "present", "body", "timeline"} {
		if contains(js, banned) {
			t.Fatalf("internal field %q leaked into JSON: %s", banned, js)
		}
	}
	for _, want := range []string{`"id"`, `"title"`, `"url"`, `"permalink"`, `"source"`, `"selected"`} {
		if !contains(js, want) {
			t.Fatalf("missing wire field %s in %s", want, js)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && (len(s) >= len(sub)) && (indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
