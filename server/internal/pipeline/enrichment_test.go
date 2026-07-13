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
	in := `{"title_cn":"标题","summary_cn":"摘要","category":"tip","relevance":80,"score":75,"selected":true,"reason_cn":"首个可复用的实战范式"}`
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

func TestSystemPromptHasScoreBands(t *testing.T) {
	for _, band := range []string{"90-100", "75-89", "60-74", "40-59", "0-39"} {
		if !strings.Contains(enrichSystemPrompt, band) {
			t.Fatalf("score rubric missing band %q", band)
		}
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
