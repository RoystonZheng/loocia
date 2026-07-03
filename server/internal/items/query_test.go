package items

import (
	"context"
	"testing"
	"time"
)

func seed(t *testing.T, s *Store, items ...Item) {
	t.Helper()
	for _, it := range items {
		if err := s.Upsert(context.Background(), it); err != nil {
			t.Fatalf("seed Upsert %s: %v", it.ID, err)
		}
	}
}

func ids(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func TestListOrdersByPublishedDesc(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	seed(t, s,
		sampleItem("old", base),
		sampleItem("new", base.Add(48*time.Hour)),
		sampleItem("mid", base.Add(24*time.Hour)),
	)
	got, err := s.List(context.Background(), ListParams{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"new", "mid", "old"}
	if g := ids(got); !equal(g, want) {
		t.Fatalf("order: got %v want %v", g, want)
	}
}

func TestListFiltersSelectedCategorySince(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	selModel := sampleItem("sel-model", base.Add(72*time.Hour))
	selModel.Selected = true
	selModel.Category = strptr(CategoryAIModels)

	unsel := sampleItem("unsel", base.Add(72*time.Hour))
	unsel.Selected = false

	selPaperOld := sampleItem("sel-paper-old", base) // older than `since`
	selPaperOld.Selected = true
	selPaperOld.Category = strptr(CategoryPaper)

	seed(t, s, selModel, unsel, selPaperOld)

	tru := true
	// selected only
	got, _ := s.List(context.Background(), ListParams{Selected: &tru, Limit: 10})
	if g := ids(got); !equal(g, []string{"sel-model", "sel-paper-old"}) {
		t.Fatalf("selected filter: got %v", g)
	}
	// selected + category
	cat := CategoryAIModels
	got, _ = s.List(context.Background(), ListParams{Selected: &tru, Category: &cat, Limit: 10})
	if g := ids(got); !equal(g, []string{"sel-model"}) {
		t.Fatalf("category filter: got %v", g)
	}
	// selected + since (last 48h from base)
	since := base.Add(24 * time.Hour)
	got, _ = s.List(context.Background(), ListParams{Selected: &tru, Since: &since, Limit: 10})
	if g := ids(got); !equal(g, []string{"sel-model"}) {
		t.Fatalf("since filter: got %v", g)
	}
}

func TestListExcludesNotPresentAndDuplicates(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	gone := sampleItem("gone", base)
	gone.Present = false

	dup := sampleItem("dup", base)
	dup.DuplicateOfID = strptr("primary")

	keep := sampleItem("keep", base)

	seed(t, s, gone, dup, keep)

	got, _ := s.List(context.Background(), ListParams{Limit: 10})
	if g := ids(got); !equal(g, []string{"keep"}) {
		t.Fatalf("present/dup exclusion: got %v", g)
	}
}

func TestListKeysetPaginationIsCompleteAndNonOverlapping(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	// 5 items, distinct published times.
	for i := 0; i < 5; i++ {
		seed(t, s, sampleItem(string(rune('a'+i)), base.Add(time.Duration(i)*time.Hour)))
	}
	ctx := context.Background()

	// Page size 2; walk all pages via cursor.
	var seen []string
	var after *Cursor
	for {
		page, err := s.List(ctx, ListParams{Limit: 2, After: after})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(page) == 0 {
			break
		}
		seen = append(seen, ids(page)...)
		last := page[len(page)-1]
		after = &Cursor{SortKey: last.SortKey(), ID: last.ID}
		if len(page) < 2 {
			break
		}
	}
	// Newest first: e(4h) d c b a
	want := []string{"e", "d", "c", "b", "a"}
	if !equal(seen, want) {
		t.Fatalf("paginated order/coverage: got %v want %v", seen, want)
	}
}

func TestListKeysetHandlesNullPublishedAt(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	withTime := sampleItem("has-time", base)
	noTime := sampleItem("no-time", base)
	noTime.PublishedAt = nil // sorts last (epoch)
	seed(t, s, withTime, noTime)

	ctx := context.Background()
	page1, _ := s.List(ctx, ListParams{Limit: 1})
	if g := ids(page1); !equal(g, []string{"has-time"}) {
		t.Fatalf("page1: got %v", g)
	}
	after := &Cursor{SortKey: page1[0].SortKey(), ID: page1[0].ID}
	page2, _ := s.List(ctx, ListParams{Limit: 1, After: after})
	if g := ids(page2); !equal(g, []string{"no-time"}) {
		t.Fatalf("page2 (null published): got %v", g)
	}
}

func TestListSearchMatchesTitleAndSummary(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	a := sampleItem("openai-item", base.Add(2*time.Hour))
	a.Title = "OpenAI ships Sora 2"

	b := sampleItem("summary-item", base.Add(1*time.Hour))
	b.Title = "Some model update"
	b.Summary = strptr("深度对比 OpenAI 与 Anthropic")

	c := sampleItem("unrelated", base)
	c.Title = "Weather report"

	seed(t, s, a, b, c)

	q := "OpenAI"
	got, err := s.List(context.Background(), ListParams{Q: &q, Limit: 10})
	if err != nil {
		t.Fatalf("List search: %v", err)
	}
	// Both a (title) and b (summary) match; ordered by published desc → a then b.
	if g := ids(got); !equal(g, []string{"openai-item", "summary-item"}) {
		t.Fatalf("search: got %v", g)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
