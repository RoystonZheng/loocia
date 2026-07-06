package pipeline

import (
	"context"
	"os"
	"testing"
	"time"

	"aihot-server/internal/ingest"
	"aihot-server/internal/items"
	"aihot-server/internal/llm"
)

func TestToItemSelectionFloor(t *testing.T) {
	raw := ingest.RawItem{ID: "r1", URL: "https://x/1", Source: "S", Title: "Orig"}
	cases := []struct {
		name        string
		llmSelected bool
		relevance   int
		wantSel     bool
	}{
		{"llm selected always wins", true, 10, true},
		{"floor at 60 inclusive", false, 60, true},
		{"above floor", false, 85, true},
		{"below floor stays out", false, 59, false},
		{"low relevance unselected", false, 20, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Enrichment{TitleCN: "标题", SummaryCN: "摘要", Category: items.CategoryAIModels, Relevance: c.relevance, Score: 50, Selected: c.llmSelected}
			it := toItem(raw, e)
			if it.Selected != c.wantSel {
				t.Fatalf("Selected: got %v want %v (llm=%v rel=%d)", it.Selected, c.wantSel, c.llmSelected, c.relevance)
			}
			// ai_selected must preserve the RAW llm verdict regardless of the floor.
			if it.AISelected == nil || *it.AISelected != c.llmSelected {
				t.Fatalf("AISelected should preserve raw verdict %v, got %v", c.llmSelected, it.AISelected)
			}
			// ai_relevance preserved.
			if it.AIRelevance == nil || *it.AIRelevance != c.relevance {
				t.Fatalf("AIRelevance: %v", it.AIRelevance)
			}
		})
	}
}

func TestProcessBatchEnrichesAndMarksProcessed(t *testing.T) {
	pool := testPool(t)
	raw, itemsStore := testStores(t, pool)
	ctx := context.Background()

	pub := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := ingest.RawItem{
		ID: ingest.RawID("https://ex.com/x"), Source: "Src", SourceKind: "rss",
		URL: "https://ex.com/x", Title: "Original English Title", PublishedAt: &pub,
	}
	if _, err := raw.InsertRaw(ctx, r); err != nil {
		t.Fatal(err)
	}

	enr := fakeEnricher{out: Enrichment{
		TitleCN: "中文标题", SummaryCN: "中文摘要", Category: "ai-models",
		Relevance: 92, Score: 88, Selected: true,
	}}
	p := NewProcessor(raw, itemsStore, enr)

	res, err := p.ProcessBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("result: %+v", res)
	}

	got, err := itemsStore.GetByID(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title != "中文标题" || got.Summary == nil || *got.Summary != "中文摘要" {
		t.Fatalf("mapped title/summary: %+v", got)
	}
	if got.TitleEN == nil || *got.TitleEN != "Original English Title" {
		t.Fatalf("title_en should hold the original: %+v", got)
	}
	if got.Category == nil || *got.Category != "ai-models" || got.Score == nil || *got.Score != 88 {
		t.Fatalf("category/score: %+v", got)
	}
	if got.AISelected == nil || !*got.AISelected || !got.Selected {
		t.Fatalf("selected flags: %+v", got)
	}
	if got.Permalink != "/items/"+r.ID {
		t.Fatalf("permalink: %q", got.Permalink)
	}
	if got.PublishedAt == nil || !got.PublishedAt.Equal(pub) {
		t.Fatalf("published_at not carried: %v", got.PublishedAt)
	}

	res2, err := p.ProcessBatch(ctx, 10)
	if err != nil {
		t.Fatalf("second ProcessBatch: %v", err)
	}
	if res2.Processed != 0 {
		t.Fatalf("second batch should process 0, got %+v", res2)
	}
}

func TestProcessBatchIsolatesEnrichFailure(t *testing.T) {
	pool := testPool(t)
	raw, itemsStore := testStores(t, pool)
	ctx := context.Background()

	if _, err := raw.InsertRaw(ctx, ingest.RawItem{
		ID: ingest.RawID("https://ex.com/a"), Source: "S", SourceKind: "rss",
		URL: "https://ex.com/a", Title: "A",
	}); err != nil {
		t.Fatal(err)
	}

	p := NewProcessor(raw, itemsStore, fakeEnricher{err: context.DeadlineExceeded})
	res, err := p.ProcessBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ProcessBatch top-level: %v", err)
	}
	if res.Processed != 0 || res.Failed != 1 || len(res.Errors) != 1 {
		t.Fatalf("result: %+v", res)
	}
	un, err := raw.ListUnprocessed(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(un) != 1 {
		t.Fatalf("failed item should remain unprocessed, got %d", len(un))
	}
}

func TestProcessBatchRealLLM(t *testing.T) {
	if os.Getenv("AIHOT_LLM_API_KEY") == "" {
		t.Skip("AIHOT_LLM_API_KEY not set")
	}
	pool := testPool(t)
	raw, itemsStore := testStores(t, pool)
	ctx := context.Background()

	if _, err := raw.InsertRaw(ctx, ingest.RawItem{
		ID: ingest.RawID("https://ex.com/openai-sora"), Source: "OpenAI Blog", SourceKind: "rss",
		URL: "https://ex.com/openai-sora", Title: "OpenAI releases Sora 2 video model",
	}); err != nil {
		t.Fatal(err)
	}

	client, err := llm.NewClientFromEnv()
	if err != nil {
		t.Fatalf("NewClientFromEnv: %v", err)
	}
	p := NewProcessor(raw, itemsStore, NewEnricher(client))
	res, err := p.ProcessBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if res.Processed != 1 {
		t.Fatalf("real LLM should process 1: %+v (errors: %v)", res, res.Errors)
	}
	got, err := itemsStore.GetByID(ctx, ingest.RawID("https://ex.com/openai-sora"))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title == "" || got.Category == nil {
		t.Fatalf("real enrichment incomplete: %+v", got)
	}
	t.Logf("real enrichment: title=%q category=%v score=%v", got.Title, *got.Category, got.Score)
}
