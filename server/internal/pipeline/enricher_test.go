package pipeline

import (
	"context"
	"errors"
	"testing"

	"aihot-server/internal/ingest"
)

type fakeLLM struct {
	reply           string
	err             error
	gotSys, gotUser string
}

func (f *fakeLLM) Complete(ctx context.Context, system, user string) (string, error) {
	f.gotSys, f.gotUser = system, user
	return f.reply, f.err
}

func TestEnricherEnrichParsesLLMOutput(t *testing.T) {
	f := &fakeLLM{reply: `{"title_cn":"标题","summary_cn":"摘要","category":"industry","relevance":5,"score":4}`}
	e := NewEnricher(f)
	body := "body"
	got, err := e.Enrich(context.Background(), ingest.RawItem{Title: "T", RawContent: &body})
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if got.TitleCN != "标题" || got.Category != "industry" || got.Score != 4 {
		t.Fatalf("enrichment: %+v", got)
	}
	if f.gotUser == "" || f.gotSys == "" {
		t.Fatal("LLM was not called with prompts")
	}
}

func TestEnricherPropagatesLLMError(t *testing.T) {
	f := &fakeLLM{err: errors.New("boom")}
	e := NewEnricher(f)
	_, err := e.Enrich(context.Background(), ingest.RawItem{Title: "T"})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
}
