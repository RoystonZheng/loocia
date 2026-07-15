package cluster

import (
	"context"
	"fmt"
	"time"

	"aihot-server/internal/items"
)

// Result summarizes one clustering+heat pass.
type Result struct {
	WindowItems int
	Clusters    int
}

// Pass runs the periodic clustering + heat recompute over a trailing window.
type Pass struct {
	items *items.Store
	store *Store
	llm   LLM
}

func NewPass(itemsStore *items.Store, store *Store, l LLM) *Pass {
	return &Pass{items: itemsStore, store: store, llm: l}
}

const maxWindowItems = 200

// Run groups same-event items in [now-window, now), assigns cluster ids /
// primaries, and rebuilds the clusters table with fresh heat. The LLM grouping
// failing aborts the pass with ZERO writes. Fewer than 2 window items → no
// grouping; assignments in the window and the clusters table are cleared.
func (p *Pass) Run(ctx context.Context, window time.Duration, now time.Time) (Result, error) {
	var res Result
	since := now.Add(-window)
	until := now.Add(time.Minute) // include items published "just now"

	its, err := p.items.List(ctx, items.ListParams{Since: &since, Until: &until, Limit: maxWindowItems})
	if err != nil {
		return res, fmt.Errorf("list window: %w", err)
	}
	res.WindowItems = len(its)

	var groups [][]items.Item
	if len(its) >= 2 {
		groups, err = llmGroup(ctx, p.llm, its)
		if err != nil {
			return res, fmt.Errorf("grouping: %w", err) // zero writes on failure
		}
	}

	// From here on we mutate: reset the window, apply assignments, rebuild clusters.
	if err := p.items.ClearClusters(ctx, since, until); err != nil {
		return res, fmt.Errorf("clear assignments: %w", err)
	}
	clusters := make([]Cluster, 0, len(groups))
	for _, members := range groups {
		c := buildCluster(members, now)
		for _, m := range members {
			if err := p.items.AssignCluster(ctx, m.ID, c.ID, m.ID == c.PrimaryItemID); err != nil {
				return res, fmt.Errorf("assign %s: %w", m.ID, err)
			}
		}
		clusters = append(clusters, c)
	}
	if err := p.store.ReplaceAll(ctx, clusters); err != nil {
		return res, fmt.Errorf("replace clusters: %w", err)
	}
	// Items that aged out of the window keep their last assignment; once their
	// cluster is gone from the rebuilt table a stale non-primary flag would fold
	// them out of the selected feed forever. Reconcile against the live set.
	live := make([]string, 0, len(clusters))
	for _, c := range clusters {
		live = append(live, c.ID)
	}
	if _, err := p.items.ClearStaleClusterRefs(ctx, live); err != nil {
		return res, fmt.Errorf("clear stale refs: %w", err)
	}
	res.Clusters = len(clusters)
	return res, nil
}
