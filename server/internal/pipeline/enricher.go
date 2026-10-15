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
	llm     LLM
	profile Profile
}

func NewEnricher(llm LLM) Enricher {
	return &llmEnricher{llm: llm, profile: Profile{Name: "AI资讯", Description: "人工智能、大模型、AI 产品、研究和行业动态", Categories: []string{"ai-models", "ai-products", "industry", "paper", "tip"}}}
}

func NewEnricherWithProfile(llm LLM, profile Profile) Enricher {
	if profile.Name == "" {
		profile.Name = "通用资讯"
	}
	if profile.Description == "" {
		profile.Description = "按来源计划收集并筛选有事实依据的信息"
	}
	if len(profile.Categories) == 0 {
		profile.Categories = []string{"general"}
	}
	return &llmEnricher{llm: llm, profile: profile}
}

func (e *llmEnricher) Enrich(ctx context.Context, r ingest.RawItem) (Enrichment, error) {
	system, user := buildPromptForProfile(r, e.profile)
	out, err := e.llm.Complete(ctx, system, user)
	if err != nil {
		return Enrichment{}, err
	}
	return parseEnrichmentForCategories(out, e.profile.Categories)
}
