package daily

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aihot-server/internal/items"
)

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

func mkSelItem(id, cat string, score int, pub time.Time) items.Item {
	summary := "摘要-" + id
	c := cat
	sc := score
	return items.Item{
		ID: id, Title: "标题-" + id, URL: "https://x/" + id, Permalink: "/items/" + id,
		Source: "Src", PublishedAt: &pub, Summary: &summary, Category: &c, Score: &sc,
		Selected: true, Present: true,
	}
}

func TestBuildSectionsGroupsAndOrders(t *testing.T) {
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)
	its := []items.Item{
		mkSelItem("p1", "paper", 70, base),
		mkSelItem("m1", "ai-models", 80, base),
		mkSelItem("m2", "ai-models", 95, base.Add(time.Hour)),
	}
	secs := buildSections(its)
	// Fixed order: ai-models before paper; empty categories skipped.
	if len(secs) != 2 || secs[0].Label != "模型发布/更新" || secs[1].Label != "论文研究" {
		t.Fatalf("section order: %+v", secs)
	}
	// Within section: score desc → m2 (95) before m1 (80).
	if secs[0].Items[0].Title != "标题-m2" || secs[0].Items[1].Title != "标题-m1" {
		t.Fatalf("in-section order: %+v", secs[0].Items)
	}
	// Field mapping.
	it0 := secs[0].Items[0]
	if it0.Summary != "摘要-m2" || it0.SourceURL != "https://x/m2" || it0.SourceName != "Src" {
		t.Fatalf("mapping: %+v", it0)
	}
	if it0.Permalink == nil || *it0.Permalink != "/items/m2" {
		t.Fatalf("permalink: %+v", it0)
	}
}

func TestBuildSectionsSkipsUncategorized(t *testing.T) {
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)
	noCat := mkSelItem("x", "ai-models", 50, base)
	noCat.Category = nil
	secs := buildSections([]items.Item{noCat})
	if len(secs) != 0 {
		t.Fatalf("uncategorized should be skipped: %+v", secs)
	}
}

func TestBuildFlashesTakesMostRecent(t *testing.T) {
	base := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	var its []items.Item
	for i := 0; i < 7; i++ {
		its = append(its, mkSelItem(string(rune('a'+i)), "industry", 60, base.Add(time.Duration(i)*time.Hour)))
	}
	fl := buildFlashes(its, 5)
	if len(fl) != 5 {
		t.Fatalf("want 5 flashes, got %d", len(fl))
	}
	// Most recent first: g (6h) then f, e, d, c.
	if fl[0].Title != "标题-g" || fl[4].Title != "标题-c" {
		t.Fatalf("flash order: %+v", fl)
	}
}

type fakeLLM struct {
	reply string
	err   error
}

func (f fakeLLM) Complete(ctx context.Context, system, user string) (string, error) {
	return f.reply, f.err
}

func TestGenerateLeadParsesJSON(t *testing.T) {
	f := fakeLLM{reply: `{"title":"AI 圈今日看点","lead_paragraph":"今天模型层面动作频繁。"}`}
	lead, err := generateLead(context.Background(), f, []Section{{Label: "模型发布/更新", Items: []SectionItem{{Title: "t", Summary: "s"}}}})
	if err != nil {
		t.Fatalf("generateLead: %v", err)
	}
	if lead.Title != "AI 圈今日看点" || !strings.Contains(lead.LeadParagraph, "模型") {
		t.Fatalf("lead: %+v", lead)
	}
}

func TestGenerateLeadErrors(t *testing.T) {
	if _, err := generateLead(context.Background(), fakeLLM{err: errors.New("boom")}, nil); err == nil {
		t.Fatal("LLM error should propagate")
	}
	if _, err := generateLead(context.Background(), fakeLLM{reply: "not json"}, nil); err == nil {
		t.Fatal("garbage reply should error")
	}
	if _, err := generateLead(context.Background(), fakeLLM{reply: `{"title":"","lead_paragraph":"p"}`}, nil); err == nil {
		t.Fatal("empty title should error")
	}
}
