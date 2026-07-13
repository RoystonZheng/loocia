package cluster

import (
	"context"
	"testing"
	"time"
)

func TestReplaceAllAndListHotTopics(t *testing.T) {
	cs, is, _ := newLiveStores(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)

	seedItem(t, is, "hot1", "OpenAI Blog", 5, base, true)
	seedItem(t, is, "warm1", "机器之心", 3, base.Add(time.Hour), true)

	err := cs.ReplaceAll(ctx, []Cluster{
		{ID: "warm1", PrimaryItemID: "warm1", SourceCount: 2, SourceNames: []string{"机器之心", "量子位"},
			FirstAt: base, LatestAt: base.Add(time.Hour), Heat: 1.5},
		{ID: "hot1", PrimaryItemID: "hot1", SourceCount: 4, SourceNames: []string{"OpenAI Blog", "机器之心", "量子位", "36氪"},
			FirstAt: base, LatestAt: base, Heat: 3.7},
	})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	rows, err := cs.ListHotTopics(ctx)
	if err != nil {
		t.Fatalf("ListHotTopics: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	// heat desc → hot1 first.
	if rows[0].ID != "hot1" || rows[0].Title != "标题-hot1" || rows[0].SourceCount != 4 {
		t.Fatalf("row0: %+v", rows[0])
	}
	if len(rows[0].SourceNames) != 4 || rows[0].SourceNames[0] != "OpenAI Blog" {
		t.Fatalf("source names order: %+v", rows[0].SourceNames)
	}

	// Second ReplaceAll fully swaps content.
	if err := cs.ReplaceAll(ctx, []Cluster{}); err != nil {
		t.Fatalf("ReplaceAll empty: %v", err)
	}
	rows, err = cs.ListHotTopics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("want empty after swap, got %d", len(rows))
	}
}

func TestHeatOfDecay(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	if h := heatOf(4, now, now); h != 4 {
		t.Fatalf("fresh heat: %v", h)
	}
	if h := heatOf(4, now.Add(-24*time.Hour), now); h != 2 {
		t.Fatalf("24h-old heat should halve: %v", h)
	}
	if h := heatOf(1, now.Add(-48*time.Hour), now); h != 0.25 {
		t.Fatalf("48h-old heat: %v", h)
	}
}
