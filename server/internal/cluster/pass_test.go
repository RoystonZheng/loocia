package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	"aihot-server/internal/items"
)

func TestRunClustersAndComputesHeat(t *testing.T) {
	cs, is, _ := newLiveStores(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	base := now.Add(-4 * time.Hour)

	// Same event from two sources + one unrelated.
	seedItem(t, is, "ev1a", "OpenAI Blog", 5, base, true)
	seedItem(t, is, "ev1b", "机器之心", 4, base.Add(time.Hour), true)
	seedItem(t, is, "solo", "量子位", 3, base.Add(2*time.Hour), true)

	p := NewPass(is, cs, &fakeLLM{reply: `{"clusters":[{"item_ids":["ev1a","ev1b"]}]}`})
	res, err := p.Run(ctx, 72*time.Hour, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.WindowItems != 3 || res.Clusters != 1 {
		t.Fatalf("result: %+v", res)
	}

	// Items got assignments: ev1a primary (higher score), ev1b secondary.
	ga, _ := is.GetByID(ctx, "ev1a")
	gb, _ := is.GetByID(ctx, "ev1b")
	gs, _ := is.GetByID(ctx, "solo")
	if ga.ClusterID == nil || *ga.ClusterID != "ev1a" || ga.ClusterPrimary == nil || !*ga.ClusterPrimary {
		t.Fatalf("ev1a: %+v", ga)
	}
	if gb.ClusterID == nil || *gb.ClusterID != "ev1a" || gb.ClusterPrimary == nil || *gb.ClusterPrimary {
		t.Fatalf("ev1b: %+v", gb)
	}
	if gs.ClusterID != nil {
		t.Fatalf("solo should be unclustered: %+v", gs)
	}

	// Clusters table has the row with 2 sources and decayed heat.
	rows, err := cs.ListHotTopics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "ev1a" || rows[0].SourceCount != 2 {
		t.Fatalf("hot rows: %+v", rows)
	}

	// Selected feed now hides the secondary.
	tru := true
	feed, err := is.List(ctx, items.ListParams{Selected: &tru, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range feed {
		if it.ID == "ev1b" {
			t.Fatal("secondary leaked into selected feed")
		}
	}
}

func TestRunLLMFailureWritesNothing(t *testing.T) {
	cs, is, _ := newLiveStores(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	seedItem(t, is, "x1", "S1", 4, now.Add(-time.Hour), true)
	seedItem(t, is, "x2", "S2", 3, now.Add(-2*time.Hour), true)

	p := NewPass(is, cs, &fakeLLM{err: errors.New("llm down")})
	if _, err := p.Run(ctx, 72*time.Hour, now); err == nil {
		t.Fatal("Run should fail when grouping fails")
	}
	// No assignments, no clusters.
	g, _ := is.GetByID(ctx, "x1")
	if g.ClusterID != nil {
		t.Fatalf("no assignment expected: %+v", g)
	}
	rows, _ := cs.ListHotTopics(ctx)
	if len(rows) != 0 {
		t.Fatalf("no clusters expected: %+v", rows)
	}
}

func TestRunFewItemsClearsClusters(t *testing.T) {
	cs, is, _ := newLiveStores(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	// Stale cluster row from a previous pass.
	seedItem(t, is, "old1", "S1", 4, now.Add(-time.Hour), true)
	if err := cs.ReplaceAll(ctx, []Cluster{{ID: "old1", PrimaryItemID: "old1", SourceCount: 3,
		SourceNames: []string{"S1"}, FirstAt: now, LatestAt: now, Heat: 3}}); err != nil {
		t.Fatal(err)
	}

	p := NewPass(is, cs, &fakeLLM{reply: `ignored`})
	res, err := p.Run(ctx, 72*time.Hour, now)
	if err != nil {
		t.Fatalf("Run with 1 item: %v", err)
	}
	if res.Clusters != 0 {
		t.Fatalf("result: %+v", res)
	}
	rows, _ := cs.ListHotTopics(ctx)
	if len(rows) != 0 {
		t.Fatalf("stale clusters should be cleared: %+v", rows)
	}
}

func TestRunClearsStaleAssignmentsOutsideWindow(t *testing.T) {
	cs, is, _ := newLiveStores(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	base := now.Add(-4 * time.Hour)

	// In-window pair that the LLM groups this pass.
	seedItem(t, is, "ev1a", "OpenAI Blog", 5, base, true)
	seedItem(t, is, "ev1b", "机器之心", 4, base.Add(time.Hour), true)
	// Aged-out item carrying a stale non-primary assignment from a long-gone
	// cluster (the "orphan" that would be folded out of the selected feed).
	seedItem(t, is, "old-orphan", "量子位", 4, now.Add(-100*time.Hour), true)
	if err := is.AssignCluster(ctx, "old-orphan", "gone-cluster", false); err != nil {
		t.Fatal(err)
	}

	p := NewPass(is, cs, &fakeLLM{reply: `{"clusters":[{"item_ids":["ev1a","ev1b"]}]}`})
	if _, err := p.Run(ctx, 72*time.Hour, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The stale assignment is reconciled away; the fresh one stays.
	orphan, _ := is.GetByID(ctx, "old-orphan")
	if orphan.ClusterID != nil || orphan.ClusterPrimary != nil {
		t.Fatalf("stale assignment should be cleared: %+v", orphan)
	}
	ga, _ := is.GetByID(ctx, "ev1a")
	if ga.ClusterID == nil || *ga.ClusterID != "ev1a" {
		t.Fatalf("fresh assignment should survive: %+v", ga)
	}

	// And the orphan is visible in the selected feed again.
	tru := true
	feed, err := is.List(ctx, items.ListParams{Selected: &tru, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range feed {
		if it.ID == "old-orphan" {
			found = true
		}
	}
	if !found {
		t.Fatal("old-orphan should reappear in selected feed after reconcile")
	}
}
