package pipeline

import (
	"strings"
	"testing"

	"aihot-server/internal/ingest"
)

func TestParseEnrichmentPlainJSON(t *testing.T) {
	raw := `{"title_cn":"模型X发布","summary_cn":"简短摘要。","category":"ai-models","relevance":5,"score":4,"reason_cn":"看点"}`
	e, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment: %v", err)
	}
	if e.TitleCN != "模型X发布" || e.SummaryCN != "简短摘要。" {
		t.Fatalf("title/summary: %+v", e)
	}
	if e.Category != "ai-models" || e.Relevance != 5 || e.Score != 4 {
		t.Fatalf("fields: %+v", e)
	}
}

func TestParseEnrichmentStripsCodeFence(t *testing.T) {
	raw := "```json\n{\"title_cn\":\"标题\",\"summary_cn\":\"摘要\",\"category\":\"paper\",\"relevance\":3,\"score\":3}\n```"
	e, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment: %v", err)
	}
	if e.Category != "paper" {
		t.Fatalf("fields: %+v", e)
	}
}

func TestParseEnrichmentRejectsBadCategory(t *testing.T) {
	raw := `{"title_cn":"t","summary_cn":"s","category":"nonsense","relevance":3,"score":3}`
	_, err := parseEnrichment(raw)
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
}

func TestParseEnrichmentClampsScoreAndRelevance(t *testing.T) {
	// The model occasionally emits an old-scale or out-of-band number; clamp
	// into [1,5] instead of dropping the item.
	raw := `{"title_cn":"t","summary_cn":"s","category":"tip","relevance":90,"score":0}`
	e, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment should not fail on out-of-range: %v", err)
	}
	if e.Score != 1 || e.Relevance != 5 {
		t.Fatalf("clamp: score=%d relevance=%d, want score=1 relevance=5", e.Score, e.Relevance)
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

func TestBuildPromptCapsLongBody(t *testing.T) {
	// A full-text 公众号 body (100k+ chars) must not blow up the prompt.
	body := strings.Repeat("正", 50000)
	r := ingest.RawItem{Title: "T", RawContent: &body}
	_, user := buildPrompt(r)
	if n := len([]rune(user)); n > maxPromptBodyRunes+50 {
		t.Fatalf("user prompt not capped: %d runes", n)
	}
	if !strings.HasSuffix(user, "…") {
		t.Fatal("truncated body should end with an ellipsis")
	}
}

func TestParseEnrichmentReadsReason(t *testing.T) {
	in := `{"title_cn":"标题","summary_cn":"摘要","category":"tip","relevance":5,"score":4,"reason_cn":"首个可复用的实战范式"}`
	e, err := parseEnrichment(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.ReasonCN != "首个可复用的实战范式" {
		t.Fatalf("reason: %q", e.ReasonCN)
	}
}

func TestSystemPromptListsReason(t *testing.T) {
	if !strings.Contains(enrichSystemPrompt, "reason_cn") {
		t.Fatal("system prompt should ask for reason_cn")
	}
}

func TestSystemPromptHasScoreTiers(t *testing.T) {
	// Five ordinal tiers, not the old 0-100 bands.
	for _, tier := range []string{"5", "4", "3", "2", "1"} {
		if !strings.Contains(enrichSystemPrompt, tier) {
			t.Fatalf("score rubric missing tier %q", tier)
		}
	}
	for _, kw := range []string{"工程", "行业", "深度", "营销"} {
		if !strings.Contains(enrichSystemPrompt, kw) {
			t.Fatalf("value model missing keyword %q", kw)
		}
	}
	if strings.Contains(enrichSystemPrompt, "selected") {
		t.Fatal("prompt should no longer ask for a selected field")
	}
}

func TestSystemPromptForbidsAsciiQuotes(t *testing.T) {
	// The model emitting unescaped ASCII double-quotes inside Chinese summaries
	// broke json.Unmarshal; the prompt must steer it to 「」 instead. Lock it in.
	if !strings.Contains(enrichSystemPrompt, "「」") {
		t.Fatal("system prompt should instruct the model to use 「」 for inner quotes")
	}
}

func TestTruncateRunesShortNoop(t *testing.T) {
	if got := truncateRunes("短文本", 100); got != "短文本" {
		t.Fatalf("short text should be unchanged: %q", got)
	}
}
