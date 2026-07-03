package pipeline

import (
	"strings"
	"testing"

	"aihot-server/internal/ingest"
)

func TestParseEnrichmentPlainJSON(t *testing.T) {
	raw := `{"title_cn":"模型X发布","summary_cn":"简短摘要。","category":"ai-models","relevance":90,"score":85,"selected":true}`
	e, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment: %v", err)
	}
	if e.TitleCN != "模型X发布" || e.SummaryCN != "简短摘要。" {
		t.Fatalf("title/summary: %+v", e)
	}
	if e.Category != "ai-models" || e.Relevance != 90 || e.Score != 85 || !e.Selected {
		t.Fatalf("fields: %+v", e)
	}
}

func TestParseEnrichmentStripsCodeFence(t *testing.T) {
	raw := "```json\n{\"title_cn\":\"标题\",\"summary_cn\":\"摘要\",\"category\":\"paper\",\"relevance\":70,\"score\":60,\"selected\":false}\n```"
	e, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment: %v", err)
	}
	if e.Category != "paper" || e.Selected {
		t.Fatalf("fields: %+v", e)
	}
}

func TestParseEnrichmentRejectsBadCategory(t *testing.T) {
	raw := `{"title_cn":"t","summary_cn":"s","category":"nonsense","relevance":50,"score":50,"selected":false}`
	_, err := parseEnrichment(raw)
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
}

func TestParseEnrichmentRejectsOutOfRangeScore(t *testing.T) {
	raw := `{"title_cn":"t","summary_cn":"s","category":"tip","relevance":50,"score":150,"selected":false}`
	_, err := parseEnrichment(raw)
	if err == nil {
		t.Fatal("expected error for score>100")
	}
}

func TestBuildPromptIncludesTitleAndBody(t *testing.T) {
	body := "some body text"
	r := ingest.RawItem{Title: "Original Title", RawContent: &body}
	sys, user := buildPrompt(r)
	if !strings.Contains(sys, "ai-models") {
		t.Fatal("system prompt should enumerate the category slugs")
	}
	if !strings.Contains(user, "Original Title") || !strings.Contains(user, "some body text") {
		t.Fatalf("user prompt missing title/body: %q", user)
	}
}
