package daily

import (
	"context"
	"errors"
	"testing"
	"time"

	"aihot-server/internal/items"
)

// seedItems inserts selected items into the live items table for the window.
func seedItems(t *testing.T, itemsStore *items.Store, its ...items.Item) {
	t.Helper()
	for _, it := range its {
		if err := itemsStore.Upsert(context.Background(), it); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func newLiveStores(t *testing.T) (*Store, *items.Store) {
	t.Helper()
	s := newTestStore(t) // daily store, schema + truncate (skips w/o DSN)
	itemsStore := items.New(s.pool)
	if err := itemsStore.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("items EnsureSchema: %v", err)
	}
	if _, err := s.pool.Exec(context.Background(), "TRUNCATE items"); err != nil {
		t.Fatalf("truncate items: %v", err)
	}
	return s, itemsStore
}

func TestGenerateBuildsAndStoresReport(t *testing.T) {
	s, itemsStore := newLiveStores(t)
	ctx := context.Background()
	day := time.Date(2026, 5, 7, 0, 0, 0, 0, beijing)

	seedItems(t, itemsStore,
		mkSelItem("in1", "ai-models", 90, day.Add(2*time.Hour)),
		mkSelItem("in2", "industry", 75, day.Add(20*time.Hour)),
		mkSelItem("out-after", "ai-models", 99, day.Add(25*time.Hour)), // outside window
	)
	// Unselected item in window must be excluded.
	unsel := mkSelItem("unsel", "ai-models", 88, day.Add(3*time.Hour))
	unsel.Selected = false
	seedItems(t, itemsStore, unsel)

	g := NewGenerator(itemsStore, s, fakeLLM{reply: `{"title":"今日AI","lead_paragraph":"导语。"}`})
	now := day.Add(24*time.Hour + time.Hour)
	rep, err := g.Generate(ctx, "2026-05-07", now)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if rep.Date != "2026-05-07" || !rep.WindowStart.Equal(day) || !rep.WindowEnd.Equal(day.Add(24*time.Hour)) {
		t.Fatalf("window: %+v", rep)
	}
	if rep.Lead == nil || rep.Lead.Title != "今日AI" {
		t.Fatalf("lead: %+v", rep.Lead)
	}
	// Only the 2 in-window selected items; out-after and unsel excluded.
	total := 0
	for _, sec := range rep.Sections {
		total += len(sec.Items)
	}
	if total != 2 || len(rep.Sections) != 2 {
		t.Fatalf("sections: %+v", rep.Sections)
	}
	if len(rep.Flashes) != 2 {
		t.Fatalf("flashes: %+v", rep.Flashes)
	}

	// Stored and readable.
	got, err := s.GetByDate(ctx, "2026-05-07")
	if err != nil {
		t.Fatalf("GetByDate after generate: %v", err)
	}
	if got.Lead == nil || got.Lead.Title != "今日AI" {
		t.Fatalf("stored lead: %+v", got.Lead)
	}
}

func TestGenerateLLMFailureYieldsNilLead(t *testing.T) {
	s, itemsStore := newLiveStores(t)
	day := time.Date(2026, 5, 7, 0, 0, 0, 0, beijing)
	seedItems(t, itemsStore, mkSelItem("in1", "ai-models", 90, day.Add(2*time.Hour)))

	g := NewGenerator(itemsStore, s, fakeLLM{err: errors.New("llm down")})
	rep, err := g.Generate(context.Background(), "2026-05-07", day.Add(25*time.Hour))
	if err != nil {
		t.Fatalf("Generate should not fail on LLM error: %v", err)
	}
	if rep.Lead != nil {
		t.Fatalf("lead should be nil on LLM failure: %+v", rep.Lead)
	}
	if len(rep.Sections) != 1 {
		t.Fatalf("sections should still build: %+v", rep.Sections)
	}
}

func TestGenerateNoItemsReturnsErrNoItems(t *testing.T) {
	s, itemsStore := newLiveStores(t)
	g := NewGenerator(itemsStore, s, fakeLLM{reply: `{}`})
	_, err := g.Generate(context.Background(), "2026-05-07", time.Date(2026, 5, 8, 1, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrNoItems) {
		t.Fatalf("want ErrNoItems, got %v", err)
	}
	// Nothing stored → GetByDate 404s.
	if _, err := s.GetByDate(context.Background(), "2026-05-07"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no report should be stored: %v", err)
	}
}

func TestGenerateRejectsBadDate(t *testing.T) {
	s, itemsStore := newLiveStores(t)
	g := NewGenerator(itemsStore, s, fakeLLM{})
	if _, err := g.Generate(context.Background(), "07/05/2026", time.Now()); err == nil {
		t.Fatal("bad date should error")
	}
}
