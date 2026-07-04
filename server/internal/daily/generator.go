package daily

import (
	"context"
	"errors"
	"fmt"
	"time"

	"aihot-server/internal/items"
)

// ErrNoItems means the window had no selected items; no report is stored.
var ErrNoItems = errors.New("daily: no selected items in window")

const (
	maxWindowItems = 200
	flashCount     = 5
)

// Generator builds and stores one day's report.
type Generator struct {
	items *items.Store
	store *Store
	llm   LLM
}

func NewGenerator(itemsStore *items.Store, store *Store, l LLM) *Generator {
	return &Generator{items: itemsStore, store: store, llm: l}
}

// Generate builds the report for the UTC day `date` (YYYY-MM-DD), stores it,
// and returns it. The LLM writes only the lead; on LLM failure the report is
// stored with a nil lead. Zero selected items → ErrNoItems, nothing stored.
func (g *Generator) Generate(ctx context.Context, date string, now time.Time) (*Report, error) {
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("bad date %q: %w", date, err)
	}
	ws := day.UTC()
	we := ws.Add(24 * time.Hour)

	tru := true
	its, err := g.items.List(ctx, items.ListParams{
		Selected: &tru, Since: &ws, Until: &we, Limit: maxWindowItems,
	})
	if err != nil {
		return nil, fmt.Errorf("list window items: %w", err)
	}
	if len(its) == 0 {
		return nil, ErrNoItems
	}

	secs := buildSections(its)
	rep := Report{
		Date:        date,
		GeneratedAt: now.UTC(),
		WindowStart: ws,
		WindowEnd:   we,
		Sections:    secs,
		Flashes:     buildFlashes(its, flashCount),
	}
	// Lead is best-effort: a model failure must not sink the daily.
	if lead, err := generateLead(ctx, g.llm, secs); err == nil {
		rep.Lead = lead
	}

	if err := g.store.Upsert(ctx, rep); err != nil {
		return nil, fmt.Errorf("store report: %w", err)
	}
	return &rep, nil
}
