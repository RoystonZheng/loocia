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
