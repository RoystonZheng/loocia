package pipeline

import (
	"context"

	"aihot-server/internal/ingest"
)

// LLM is the minimal completion surface the pipeline needs (satisfied by *llm.Client).
type LLM interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// Enricher turns a raw item into structured enrichment.
type Enricher interface {
	Enrich(ctx context.Context, r ingest.RawItem) (Enrichment, error)
}

type llmEnricher struct {
	llm LLM
}

func NewEnricher(llm LLM) Enricher {
	return &llmEnricher{llm: llm}
}

func (e *llmEnricher) Enrich(ctx context.Context, r ingest.RawItem) (Enrichment, error) {
	system, user := buildPrompt(r)
	out, err := e.llm.Complete(ctx, system, user)
	if err != nil {
		return Enrichment{}, err
	}
	return parseEnrichment(out)
}
