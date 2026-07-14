package pipeline

import (
	"context"
	"fmt"
	"os"
	"regexp"
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

// PageResolver fetches a page once and fills missing media plus the extracted
// full article body. Best-effort: nil pointers when nothing found. Implemented
// by ingest.PageResolver.
type PageResolver interface {
	Resolve(ctx context.Context, pageURL string) (image, video, article *string)
}

// Processor enriches unprocessed raw items into the items table.
type Processor struct {
	raw        *ingest.RawStore
	items      *items.Store
	enr        Enricher
	page       PageResolver  // optional; nil disables page media/body backfill
	translator Translator    // optional; nil disables translation
	termsX     TermExtractor // optional; nil disables term extraction
	termsSink  TermsSink     // where extracted terms go (item_terms)
}

func NewProcessor(raw *ingest.RawStore, itemsStore *items.Store, enr Enricher) *Processor {
	return &Processor{raw: raw, items: itemsStore, enr: enr}
}

// WithPageResolver enables page-based backfill: missing media for items lacking
// a feed image, and (for rss) replacing the feed's short snippet body with the
// full extracted article.
func (p *Processor) WithPageResolver(r PageResolver) *Processor {
	p.page = r
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

// betterBody reports whether an extracted article should replace the current
// body: it must be substantial (>=300 text chars) and clearly longer than what
// we have (>=1.5x), so a weak extraction never overwrites a decent snippet.
func betterBody(extracted string, cur *string) bool {
	n := textLen(extracted)
	if n < 300 {
		return false
	}
	if cur == nil {
		return true
	}
	return n >= textLen(*cur)*3/2
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

// textLen is the visible text length (tags stripped), counted in runes.
func textLen(s string) int {
	return len([]rune(strings.TrimSpace(tagRe.ReplaceAllString(s, ""))))
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
	// Fetch the article page once: backfill missing media, and for English
	// (rss) sources replace the feed's short snippet body with the full
	// extracted article. Best-effort — misses just leave the item as-is.
	if p.page != nil {
		img, vid, article := p.page.Resolve(ctx, r.URL)
		if it.ImageURL == nil {
			it.ImageURL = img
		}
		if it.VideoURL == nil {
			it.VideoURL = vid
		}
		if r.SourceKind == "rss" && article != nil && betterBody(*article, it.Body) {
			it.Body = article
		}
	}
	// Translate English (rss) bodies to Chinese, best-effort: a failure just
	// leaves body_cn nil and the detail page falls back to the original.
	if p.translator != nil && it.Body != nil && shouldTranslate(r.SourceKind, *it.Body) {
		if cn, err := p.translator.Translate(ctx, *it.Body); err == nil && strings.TrimSpace(cn) != "" {
			it.BodyCN = &cn
			m := p.translator.Model()
			it.BodyCNModel = &m
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
