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

// List must not fetch the full body (perf: avoid detoasting large text for every
// paged row; the public DTO omits it and no List caller reads it). GetByID still must.
func TestListOmitsBodyButGetByIDKeepsIt(t *testing.T) {
	s := newTestStore(t)
	body := "full article body — should never be read by List"
	it := sampleItem("withbody", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	it.Body = &body
	seed(t, s, it)

	got, err := s.List(context.Background(), ListParams{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].Body != nil {
		t.Fatalf("List should not populate Body, got %q", *got[0].Body)
	}
	// Sanity: other fields still come through.
	if got[0].Title != "title-withbody" {
		t.Fatalf("List dropped a real field: title=%q", got[0].Title)
	}

	full, err := s.GetByID(context.Background(), "withbody")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if full.Body == nil || *full.Body != body {
		t.Fatalf("GetByID must keep body; got %v", full.Body)
	}
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

func TestListKeysetTiebreakOnEqualPublishedAt(t *testing.T) {
	s := newTestStore(t)
	ts := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	// Four items, SAME published_at, distinct ids. Order must fall back to id DESC.
	seed(t, s,
		sampleItem("i1", ts), sampleItem("i2", ts),
		sampleItem("i3", ts), sampleItem("i4", ts),
	)
	ctx := context.Background()
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
	// same timestamp → pure id DESC, no dup across the page boundary
	if !equal(seen, []string{"i4", "i3", "i2", "i1"}) {
		t.Fatalf("equal-timestamp tiebreak paging: got %v", seen)
	}
}

func TestListKeysetPagesAmongMultipleNullPublished(t *testing.T) {
	s := newTestStore(t)
	// Two items, both NULL published_at → both sort at epoch, tiebreak by id DESC.
	n1 := sampleItem("n1", time.Time{})
	n1.PublishedAt = nil
	n2 := sampleItem("n2", time.Time{})
	n2.PublishedAt = nil
	seed(t, s, n1, n2)

	ctx := context.Background()
	page1, err := s.List(ctx, ListParams{Limit: 1})
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if g := ids(page1); !equal(g, []string{"n2"}) { // id DESC → n2 first
		t.Fatalf("page1 among nulls: got %v", g)
	}
	// build the cursor FROM a null-published row (SortKey() == epoch)
	after := &Cursor{SortKey: page1[0].SortKey(), ID: page1[0].ID}
	page2, err := s.List(ctx, ListParams{Limit: 1, After: after})
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if g := ids(page2); !equal(g, []string{"n1"}) {
		t.Fatalf("page2 among nulls (cursor from null row): got %v", g)
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

func TestSelectedModeHidesClusterSecondaries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	primary := sampleItem("prim", base.Add(2*time.Hour))
	primary.Selected = true
	secondary := sampleItem("sec", base.Add(time.Hour))
	secondary.Selected = true
	loner := sampleItem("loner", base)
	loner.Selected = true
	seed(t, s, primary, secondary, loner)

	// Cluster prim+sec with prim as primary.
	if err := s.AssignCluster(ctx, "prim", "prim", true); err != nil {
		t.Fatalf("AssignCluster prim: %v", err)
	}
	if err := s.AssignCluster(ctx, "sec", "prim", false); err != nil {
		t.Fatalf("AssignCluster sec: %v", err)
	}

	tru := true
	got, err := s.List(ctx, ListParams{Selected: &tru, Limit: 10})
	if err != nil {
		t.Fatalf("List selected: %v", err)
	}
	if g := ids(got); !equal(g, []string{"prim", "loner"}) {
		t.Fatalf("selected should hide secondary: got %v", g)
	}

	// mode=all (Selected nil) still shows the secondary.
	all, err := s.List(ctx, ListParams{Limit: 10})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if g := ids(all); !equal(g, []string{"prim", "sec", "loner"}) {
		t.Fatalf("all should include secondary: got %v", g)
	}
}

func TestClearClustersResetsWindow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	in := sampleItem("cin", base.Add(time.Hour))
	out := sampleItem("cout", base.Add(48*time.Hour))
	seed(t, s, in, out)
	for _, id := range []string{"cin", "cout"} {
		if err := s.AssignCluster(ctx, id, "cin", id == "cin"); err != nil {
			t.Fatal(err)
		}
	}
	// Clear only the first day's window.
	if err := s.ClearClusters(ctx, base, base.Add(24*time.Hour)); err != nil {
		t.Fatalf("ClearClusters: %v", err)
	}
	gin, err := s.GetByID(ctx, "cin")
	if err != nil {
		t.Fatal(err)
	}
	if gin.ClusterID != nil || gin.ClusterPrimary != nil {
		t.Fatalf("cin should be cleared: %+v", gin)
	}
	gout, err := s.GetByID(ctx, "cout")
	if err != nil {
		t.Fatal(err)
	}
	if gout.ClusterID == nil {
		t.Fatalf("cout (outside window) should keep its cluster: %+v", gout)
	}
}

func TestListUntilExcludesLaterItems(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	seed(t, s,
		sampleItem("in-window", base.Add(2*time.Hour)),
		sampleItem("at-bound", base.Add(24*time.Hour)), // == until → excluded (half-open)
		sampleItem("after", base.Add(30*time.Hour)),
	)
	until := base.Add(24 * time.Hour)
	got, err := s.List(context.Background(), ListParams{Since: &base, Until: &until, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if g := ids(got); !equal(g, []string{"in-window"}) {
		t.Fatalf("until window: got %v", g)
	}
}
