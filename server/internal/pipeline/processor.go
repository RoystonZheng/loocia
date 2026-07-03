package pipeline

import (
	"context"
	"fmt"

	"aihot-server/internal/ingest"
	"aihot-server/internal/items"
)

// Result summarizes one processing batch.
type Result struct {
	Processed int
	Failed    int
	Errors    []error
}

// Processor enriches unprocessed raw items into the items table.
type Processor struct {
	raw   *ingest.RawStore
	items *items.Store
	enr   Enricher
}

func NewProcessor(raw *ingest.RawStore, itemsStore *items.Store, enr Enricher) *Processor {
	return &Processor{raw: raw, items: itemsStore, enr: enr}
}

// ProcessBatch enriches up to limit unprocessed raw items. A single item's
// failure is isolated (collected into Result.Errors, raw row left unprocessed
// for retry) and does not abort the batch. A top-level error is returned only
// on an unexpected store failure while listing.
func (p *Processor) ProcessBatch(ctx context.Context, limit int) (Result, error) {
	var res Result
	batch, err := p.raw.ListUnprocessed(ctx, limit)
	if err != nil {
		return res, err
	}
	for _, r := range batch {
		if err := p.processOne(ctx, r); err != nil {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Errorf("item %s: %w", r.ID, err))
			continue
		}
		res.Processed++
	}
	return res, nil
}

func (p *Processor) processOne(ctx context.Context, r ingest.RawItem) error {
	e, err := p.enr.Enrich(ctx, r)
	if err != nil {
		return err
	}
	if err := p.items.Upsert(ctx, toItem(r, e)); err != nil {
		return err
	}
	return p.raw.MarkProcessed(ctx, r.ID)
}

// toItem maps a raw item + enrichment into the items row model.
func toItem(r ingest.RawItem, e Enrichment) items.Item {
	category := e.Category
	relevance := e.Relevance
	score := e.Score
	selected := e.Selected
	summary := e.SummaryCN

	it := items.Item{
		ID:          r.ID,
		Title:       e.TitleCN,
		URL:         r.URL,
		Permalink:   "/items/" + r.ID,
		Source:      r.Source,
		SourceKind:  r.SourceKind,
		PublishedAt: r.PublishedAt,
		Summary:     &summary,
		Body:        r.RawContent,
		Category:    &category,
		Score:       &score,
		AIRelevance: &relevance,
		AISelected:  &selected,
		Selected:    selected,
		Present:     true,
	}
	if r.Title != "" && r.Title != e.TitleCN {
		orig := r.Title
		it.TitleEN = &orig
	}
	return it
}
