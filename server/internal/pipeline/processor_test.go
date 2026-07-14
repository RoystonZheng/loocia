package pipeline

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"aihot-server/internal/ingest"
	"aihot-server/internal/items"
	"aihot-server/internal/llm"
)

type fakeTranslator struct {
	out string
	err error
}

func (f fakeTranslator) Translate(ctx context.Context, body string) (string, error) {
	return f.out, f.err
}

func (f fakeTranslator) Model() string { return "fake-model" }

type fakePage struct{ img, vid, article *string }

func (f fakePage) Resolve(ctx context.Context, u string) (image, video, article *string) {
	return f.img, f.vid, f.article
}

func TestBetterBody(t *testing.T) {
	short := "one short line"
	long := strings.Repeat("full article text ", 40)
	if !betterBody(long, &short) {
		t.Fatal("long extract should replace a short snippet")
	}
	if betterBody("tiny", &short) {
		t.Fatal("too-short extract must not replace")
	}
	existing := strings.Repeat("existing long body words ", 50)
	if betterBody("also fairly short here", &existing) {
		t.Fatal("extract must be >=1.5x current text length")
	}
	if !betterBody(long, nil) {
		t.Fatal("long extract should replace nil body")
	}
}

func TestShouldTranslate(t *testing.T) {
	if !shouldTranslate("rss", "hi") {
		t.Fatal("rss with body should translate")
	}
	if shouldTranslate("mp", "hi") {
		t.Fatal("mp should not translate")
	}
	if shouldTranslate("rss", "") {
		t.Fatal("empty body should not translate")
	}
	if shouldTranslate("rss", "   ") {
		t.Fatal("whitespace-only body should not translate")
	}
}

func TestToItemSelectionByScore(t *testing.T) {
	raw := ingest.RawItem{ID: "r1", URL: "https://x/1", Source: "S", Title: "Orig"}
	cases := []struct {
		name      string
		score     int
		wantSel   bool
		wantAISel bool
	}{
		{"S is selected + strong", 5, true, true},
		{"A is selected + strong", 4, true, true},
		{"B is selected, not strong", 3, true, false},
		{"C is not selected", 2, false, false},
		{"D is not selected", 1, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Enrichment{TitleCN: "标题", SummaryCN: "摘要", Category: items.CategoryAIModels, Relevance: 4, Score: c.score}
			it := toItem(raw, e)
			if it.Selected != c.wantSel {
				t.Fatalf("Selected: got %v want %v (score=%d)", it.Selected, c.wantSel, c.score)
			}
			// ai_selected now records the "strong pick" signal (score>=4).
			if it.AISelected == nil || *it.AISelected != c.wantAISel {
				t.Fatalf("AISelected: got %v want %v (score=%d)", it.AISelected, c.wantAISel, c.score)
			}
			if it.AIRelevance == nil || *it.AIRelevance != 4 {
				t.Fatalf("AIRelevance: %v", it.AIRelevance)
			}
		})
	}
}

func TestProcessBatchTranslatesRSSBody(t *testing.T) {
	pool := testPool(t)
	raw, itemsStore := testStores(t, pool)
	ctx := context.Background()

	body := "<p>English body.</p>"
	r := ingest.RawItem{
		ID: ingest.RawID("https://ex.com/tr"), Source: "Src", SourceKind: "rss",
		URL: "https://ex.com/tr", Title: "T", RawContent: &body,
	}
	if _, err := raw.InsertRaw(ctx, r); err != nil {
		t.Fatal(err)
	}
	enr := fakeEnricher{out: Enrichment{TitleCN: "标题", SummaryCN: "摘要", Category: "ai-models", Relevance: 5, Score: 4}}

	p := NewProcessor(raw, itemsStore, enr).WithTranslator(fakeTranslator{out: "<p>中文正文。</p>"})
	if _, err := p.ProcessBatch(ctx, 10); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	got, err := itemsStore.GetByID(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.BodyCN == nil || *got.BodyCN != "<p>中文正文。</p>" {
		t.Fatalf("BodyCN should be set on rss translation: %v", got.BodyCN)
	}
}

func TestProcessBatchUsesFullArticleForRSS(t *testing.T) {
	pool := testPool(t)
	raw, itemsStore := testStores(t, pool)
	ctx := context.Background()

	long := strings.Repeat("full extracted article body words ", 40)

	short := "short snippet"
	rss := ingest.RawItem{
		ID: ingest.RawID("https://ex.com/full-rss"), Source: "Src", SourceKind: "rss",
		URL: "https://ex.com/full-rss", Title: "T", RawContent: &short,
	}
	if _, err := raw.InsertRaw(ctx, rss); err != nil {
		t.Fatal(err)
	}

	mpBody := "short snippet"
	mp := ingest.RawItem{
		ID: ingest.RawID("https://ex.com/full-mp"), Source: "Src", SourceKind: "mp",
		URL: "https://ex.com/full-mp", Title: "T", RawContent: &mpBody,
	}
	if _, err := raw.InsertRaw(ctx, mp); err != nil {
		t.Fatal(err)
	}

	enr := fakeEnricher{out: Enrichment{TitleCN: "标题", SummaryCN: "摘要", Category: "ai-models", Relevance: 5, Score: 4}}
	p := NewProcessor(raw, itemsStore, enr).WithPageResolver(fakePage{article: &long})
	if _, err := p.ProcessBatch(ctx, 10); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	gotRSS, err := itemsStore.GetByID(ctx, rss.ID)
	if err != nil {
		t.Fatalf("GetByID rss: %v", err)
	}
	if gotRSS.Body == nil || *gotRSS.Body != long {
		t.Fatalf("rss body should be the full extracted article: %v", gotRSS.Body)
	}

	gotMP, err := itemsStore.GetByID(ctx, mp.ID)
	if err != nil {
		t.Fatalf("GetByID mp: %v", err)
	}
	if gotMP.Body == nil || *gotMP.Body != mpBody {
		t.Fatalf("mp body must stay the snippet (page not consulted for body): %v", gotMP.Body)
	}
}

func TestProcessBatchTranslationFailureStillSaves(t *testing.T) {
	pool := testPool(t)
	raw, itemsStore := testStores(t, pool)
	ctx := context.Background()

	body := "<p>English body.</p>"
	r := ingest.RawItem{
		ID: ingest.RawID("https://ex.com/trfail"), Source: "Src", SourceKind: "rss",
		URL: "https://ex.com/trfail", Title: "T", RawContent: &body,
	}
	if _, err := raw.InsertRaw(ctx, r); err != nil {
		t.Fatal(err)
	}
	enr := fakeEnricher{out: Enrichment{TitleCN: "标题", SummaryCN: "摘要", Category: "ai-models", Relevance: 5, Score: 4}}

	// Translation error is best-effort: the item is still saved with body_cn nil.
	p := NewProcessor(raw, itemsStore, enr).WithTranslator(fakeTranslator{err: errors.New("boom")})
	res, err := p.ProcessBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("translation failure must not fail the item: %+v", res)
	}
	got, err := itemsStore.GetByID(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.BodyCN != nil {
		t.Fatalf("BodyCN should be nil on translation failure: %v", got.BodyCN)
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
		Relevance: 5, Score: 4,
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
	if got.Category == nil || *got.Category != "ai-models" || got.Score == nil || *got.Score != 4 {
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
