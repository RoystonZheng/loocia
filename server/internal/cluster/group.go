package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"aihot-server/internal/items"
)

// LLM is the minimal completion surface (satisfied by *llm.Client).
type LLM interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

const groupSystemPrompt = `你是 AI 资讯编辑。给定一批资讯条目（id、标题、来源），找出「报道同一事件」的条目组。
只把确定是同一事件（同一个发布/同一件事的多源报道）的条目分到一组；不确定就不分组。
输出一个 JSON 对象（只输出 JSON）：{"clusters":[{"item_ids":["id1","id2"]}]}
规则：每组至少 2 个 id；一个 id 最多出现在一个组里；没有同事件的组就输出 {"clusters":[]}`

// llmGroup asks the LLM to group same-event items. Strict validation: unknown
// ids, overlaps, or singleton groups are errors (the pass aborts with no writes).
func llmGroup(ctx context.Context, l LLM, its []items.Item) ([][]items.Item, error) {
	byID := make(map[string]items.Item, len(its))
	var b strings.Builder
	for _, it := range its {
		byID[it.ID] = it
		fmt.Fprintf(&b, "- id: %q 标题: %s 来源: %s\n", it.ID, it.Title, it.Source)
	}

	out, err := l.Complete(ctx, groupSystemPrompt, b.String())
	if err != nil {
		return nil, err
	}

	s := strings.TrimSpace(out)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object in grouping output")
	}
	var payload struct {
		Clusters []struct {
			ItemIDs []string `json:"item_ids"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &payload); err != nil {
		return nil, fmt.Errorf("grouping json: %w", err)
	}

	seen := map[string]bool{}
	var groups [][]items.Item
	for _, c := range payload.Clusters {
		if len(c.ItemIDs) < 2 {
			return nil, fmt.Errorf("cluster with %d members (need ≥2)", len(c.ItemIDs))
		}
		var members []items.Item
		for _, id := range c.ItemIDs {
			it, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("unknown item id %q in grouping output", id)
			}
			if seen[id] {
				return nil, fmt.Errorf("item id %q appears in multiple clusters", id)
			}
			seen[id] = true
			members = append(members, it)
		}
		groups = append(groups, members)
	}
	return groups, nil
}

// choosePrimary: highest score (nil last); tie → earliest report (首报).
func choosePrimary(members []items.Item) items.Item {
	best := members[0]
	for _, it := range members[1:] {
		bs, is := -1, -1
		if best.Score != nil {
			bs = *best.Score
		}
		if it.Score != nil {
			is = *it.Score
		}
		if is > bs || (is == bs && it.SortKey().Before(best.SortKey())) {
			best = it
		}
	}
	return best
}

// buildCluster aggregates one member group into a Cluster (with heat at `now`).
func buildCluster(members []items.Item, now time.Time) Cluster {
	primary := choosePrimary(members)

	// Order members by first report to derive the source-name order.
	sorted := make([]items.Item, len(members))
	copy(sorted, members)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SortKey().Before(sorted[j].SortKey())
	})

	seen := map[string]bool{}
	var names []string
	for _, it := range sorted {
		if !seen[it.Source] {
			seen[it.Source] = true
			names = append(names, it.Source)
		}
	}
	first := sorted[0].SortKey()
	latest := sorted[len(sorted)-1].SortKey()
	return Cluster{
		ID:            primary.ID,
		PrimaryItemID: primary.ID,
		SourceCount:   len(names),
		SourceNames:   names,
		FirstAt:       first,
		LatestAt:      latest,
		Heat:          heatOf(len(names), latest, now),
	}
}
