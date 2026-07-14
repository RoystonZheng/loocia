package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"aihot-server/internal/ingest"
	"aihot-server/internal/terms"
)

type fakeExtractor struct {
	out Terms
	err error
}

func (f fakeExtractor) Extract(ctx context.Context, title, summary string) (Terms, error) {
	return f.out, f.err
}

type failingSink struct{}

func (failingSink) ReplaceForItem(ctx context.Context, itemID string, ts []terms.Term) error {
	return errors.New("sink down")
}

func seedRaw(t *testing.T, raw *ingest.RawStore, id string) {
	t.Helper()
	now := time.Now().UTC()
	body := "body"
	if _, err := raw.InsertRaw(context.Background(), ingest.RawItem{
		ID: id, Source: "Example", SourceKind: "rss", URL: "https://example.com/" + id,
		Title: "title-" + id, RawContent: &body, PublishedAt: &now,
	}); err != nil {
		t.Fatalf("insert raw: %v", err)
	}
}

func validEnrichment() Enrichment {
	return Enrichment{TitleCN: "中文标题", SummaryCN: "摘要", Category: "ai-models", Relevance: 5, Score: 3}
}

func TestProcessorWritesTerms(t *testing.T) {
	pool := testPool(t)
	raw, its := testStores(t, pool)
	ts := terms.NewStore(pool)
	if err := ts.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("terms schema: %v", err)
	}
	seedRaw(t, raw, "x1")

	proc := NewProcessor(raw, its, fakeEnricher{out: validEnrichment()}).
		WithTermExtractor(fakeExtractor{out: Terms{Entities: []string{"OpenAI"}, Topics: []string{"开源"}}}, ts)
	res, err := proc.ProcessBatch(context.Background(), 10)
	if err != nil || res.Processed != 1 {
		t.Fatalf("batch: %+v err=%v", res, err)
	}

	cloud, err := ts.Cloud(context.Background(), nil, 10)
	if err != nil {
		t.Fatalf("cloud: %v", err)
	}
	if len(cloud) != 2 {
		t.Fatalf("terms not written: %+v", cloud)
	}
}

func TestProcessorTermsFailureIsBestEffort(t *testing.T) {
	pool := testPool(t)
	raw, its := testStores(t, pool)
	ts := terms.NewStore(pool)
	if err := ts.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("terms schema: %v", err)
	}
	seedRaw(t, raw, "x2")

	proc := NewProcessor(raw, its, fakeEnricher{out: validEnrichment()}).
		WithTermExtractor(fakeExtractor{err: errors.New("llm down")}, ts)
	res, err := proc.ProcessBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("batch err: %v", err)
	}
	// 抽取挂了，item 主流程必须照常成功
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("batch: %+v", res)
	}
	it, err := its.GetByID(context.Background(), "x2")
	if err != nil || it == nil {
		t.Fatalf("item not upserted: %v", err)
	}
}

func TestProcessorSinkFailureIsBestEffort(t *testing.T) {
	pool := testPool(t)
	raw, its := testStores(t, pool)
	seedRaw(t, raw, "x3")

	// 抽取成功但落库挂了：item 主流程照常成功
	proc := NewProcessor(raw, its, fakeEnricher{out: validEnrichment()}).
		WithTermExtractor(fakeExtractor{out: Terms{Entities: []string{"OpenAI"}}}, failingSink{})
	res, err := proc.ProcessBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("batch err: %v", err)
	}
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("batch: %+v", res)
	}
	it, err := its.GetByID(context.Background(), "x3")
	if err != nil || it == nil {
		t.Fatalf("item not upserted: %v", err)
	}
}
