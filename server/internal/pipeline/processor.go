package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aihot-server/internal/ingest"
	"aihot-server/internal/items"
	"aihot-server/internal/terms"
)

// Result summarizes one processing batch.
type Result struct {
	Processed int
	Failed    int
	Errors    []error
}

// MediaResolver fills missing media (image/video) for an item by inspecting its
// article page — e.g. Open Graph tags. Best-effort: returns nil pointers when it
// finds nothing. Implemented by ingest.OGResolver.
type MediaResolver interface {
	Resolve(ctx context.Context, pageURL string) (image, video *string)
}

// Processor enriches unprocessed raw items into the items table.
type Processor struct {
	raw        *ingest.RawStore
	items      *items.Store
	enr        Enricher
	media      MediaResolver // optional; nil disables OG media backfill
	translator Translator    // optional; nil disables translation
	termsX     TermExtractor // optional; nil disables term extraction
	termsSink  TermsSink     // where extracted terms go (item_terms)
}

func NewProcessor(raw *ingest.RawStore, itemsStore *items.Store, enr Enricher) *Processor {
	return &Processor{raw: raw, items: itemsStore, enr: enr}
}

// WithMediaResolver enables OG media backfill for items lacking a feed image.
func (p *Processor) WithMediaResolver(m MediaResolver) *Processor {
	p.media = m
	return p
}

// WithTranslator enables Chinese translation of English (rss) bodies.
func (p *Processor) WithTranslator(t Translator) *Processor {
	p.translator = t
	return p
}

// TermsSink is the write surface term extraction needs (satisfied by *terms.Store).
type TermsSink interface {
	ReplaceForItem(ctx context.Context, itemID string, ts []terms.Term) error
}

// WithTermExtractor enables entity/topic extraction into the terms sink.
func (p *Processor) WithTermExtractor(x TermExtractor, sink TermsSink) *Processor {
	p.termsX = x
	p.termsSink = sink
	return p
}

// shouldTranslate reports whether an item's body should be machine-translated:
// English (rss) sources with a non-empty body. MP bodies are already Chinese.
func shouldTranslate(sourceKind, body string) bool {
	return sourceKind == "rss" && strings.TrimSpace(body) != ""
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
	it := toItem(r, e)
	// Backfill media from the article's Open Graph tags when the feed gave us
	// no image (many sources ship image-less RSS). Best-effort — a resolver
	// miss or network error just leaves the item as-is.
	if p.media != nil && it.ImageURL == nil {
		if img, vid := p.media.Resolve(ctx, r.URL); img != nil || vid != nil {
			if it.ImageURL == nil {
				it.ImageURL = img
			}
			if it.VideoURL == nil {
				it.VideoURL = vid
			}
		}
	}
	// Translate English (rss) bodies to Chinese, best-effort: a failure just
	// leaves body_cn nil and the detail page falls back to the original.
	if p.translator != nil && it.Body != nil && shouldTranslate(r.SourceKind, *it.Body) {
		if cn, err := p.translator.Translate(ctx, *it.Body); err == nil && strings.TrimSpace(cn) != "" {
			it.BodyCN = &cn
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "translate %s: %v\n", r.ID, err)
		}
	}
	if err := p.items.Upsert(ctx, it); err != nil {
		return err
	}
	// Extract entities/topics into item_terms, best-effort: extraction or the
	// sink failing must never fail the item (backfillterms can repair later).
	// Runs after Upsert because item_terms has an FK on items(id). An empty
	// extraction writes nothing — never wipe previously-good terms.
	if p.termsX != nil && p.termsSink != nil {
		summary := ""
		if it.Summary != nil {
			summary = *it.Summary
		}
		if ts, err := p.termsX.Extract(ctx, it.Title, summary); err != nil {
			fmt.Fprintf(os.Stderr, "terms %s: %v\n", r.ID, err)
		} else if rows := termRows(ts); len(rows) > 0 {
			if err := p.termsSink.ReplaceForItem(ctx, it.ID, rows); err != nil {
				fmt.Fprintf(os.Stderr, "terms store %s: %v\n", r.ID, err)
			}
		}
	}
	return p.raw.MarkProcessed(ctx, r.ID)
}

// toItem maps a raw item + enrichment into the items row model.
func toItem(r ingest.RawItem, e Enrichment) items.Item {
	category := e.Category
	relevance := e.Relevance
	score := e.Score
	// 精选门槛 = B 档及以上 (score>=3). ai_selected now records the stronger
	// "would I feature this" signal (A/S, score>=4) for analytics.
	selected := score >= 3
	aiSelected := score >= 4
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
		ImageURL:    r.ImageURL,
		VideoURL:    r.VideoURL,
		Category:    &category,
		Score:       &score,
		AIRelevance: &relevance,
		AISelected:  &aiSelected,
		Selected:    selected,
		Present:     true,
	}
	if r.Title != "" && r.Title != e.TitleCN {
		orig := r.Title
		it.TitleEN = &orig
	}
	if reason := strings.TrimSpace(e.ReasonCN); reason != "" {
		it.Reason = &reason
	}
	return it
}
