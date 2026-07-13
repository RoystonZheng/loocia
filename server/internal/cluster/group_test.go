package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aihot-server/internal/items"
)

type fakeLLM struct {
	reply   string
	err     error
	gotUser string
}

func (f *fakeLLM) Complete(ctx context.Context, system, user string) (string, error) {
	f.gotUser = user
	return f.reply, f.err
}

func mkIt(id, source string, score int, pub time.Time) items.Item {
	sc := score
	return items.Item{ID: id, Title: "标题-" + id, URL: "https://x/" + id, Permalink: "/items/" + id,
		Source: source, PublishedAt: &pub, Score: &sc, Selected: true, Present: true}
}

func TestLLMGroupParsesClusters(t *testing.T) {
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)
	its := []items.Item{mkIt("a", "S1", 5, base), mkIt("b", "S2", 4, base.Add(time.Hour)), mkIt("c", "S3", 3, base)}
	f := &fakeLLM{reply: `{"clusters":[{"item_ids":["a","b"]}]}`}
	groups, err := llmGroup(context.Background(), f, its)
	if err != nil {
		t.Fatalf("llmGroup: %v", err)
	}
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Fatalf("groups: %+v", groups)
	}
	// Prompt carried ids + titles + sources.
	for _, want := range []string{`"a"`, "标题-a", "S1"} {
		if !strings.Contains(f.gotUser, want) {
			t.Fatalf("prompt missing %s: %s", want, f.gotUser)
		}
	}
}

func TestLLMGroupStrictValidation(t *testing.T) {
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)
	its := []items.Item{mkIt("a", "S1", 5, base), mkIt("b", "S2", 4, base)}

	// Unknown id → error.
	f := &fakeLLM{reply: `{"clusters":[{"item_ids":["a","zzz"]}]}`}
	if _, err := llmGroup(context.Background(), f, its); err == nil {
		t.Fatal("unknown id should error")
	}
	// Overlapping clusters → error.
	f = &fakeLLM{reply: `{"clusters":[{"item_ids":["a","b"]},{"item_ids":["b","a"]}]}`}
	if _, err := llmGroup(context.Background(), f, its); err == nil {
		t.Fatal("overlapping clusters should error")
	}
	// Singleton cluster → error (must have ≥2 members).
	f = &fakeLLM{reply: `{"clusters":[{"item_ids":["a"]}]}`}
	if _, err := llmGroup(context.Background(), f, its); err == nil {
		t.Fatal("singleton cluster should error")
	}
	// Garbage → error.
	f = &fakeLLM{reply: `not json at all`}
	if _, err := llmGroup(context.Background(), f, its); err == nil {
		t.Fatal("garbage should error")
	}
	// LLM transport error propagates.
	f = &fakeLLM{err: errors.New("boom")}
	if _, err := llmGroup(context.Background(), f, its); err == nil {
		t.Fatal("LLM error should propagate")
	}
	// No same-event groups (empty clusters) is VALID → zero groups.
	f = &fakeLLM{reply: `{"clusters":[]}`}
	groups, err := llmGroup(context.Background(), f, its)
	if err != nil || len(groups) != 0 {
		t.Fatalf("empty clusters should be ok: %v %v", groups, err)
	}
}

func TestChoosePrimaryScoreThenEarliest(t *testing.T) {
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)
	// b has highest score → primary.
	p := choosePrimary([]items.Item{mkIt("a", "S1", 3, base), mkIt("b", "S2", 5, base.Add(time.Hour))})
	if p.ID != "b" {
		t.Fatalf("primary by score: %s", p.ID)
	}
	// Tie on score → earliest report (首报) wins.
	p = choosePrimary([]items.Item{mkIt("late", "S1", 4, base.Add(2*time.Hour)), mkIt("early", "S2", 4, base)})
	if p.ID != "early" {
		t.Fatalf("primary by 首报: %s", p.ID)
	}
}

func TestBuildClusterAggregates(t *testing.T) {
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)
	members := []items.Item{
		mkIt("m2", "量子位", 4, base.Add(2*time.Hour)),
		mkIt("m1", "机器之心", 5, base),
		mkIt("m3", "机器之心", 2, base.Add(3*time.Hour)), // duplicate source
	}
	now := base.Add(3 * time.Hour)
	c := buildCluster(members, now)
	if c.PrimaryItemID != "m1" || c.ID != "m1" {
		t.Fatalf("primary: %+v", c)
	}
	if c.SourceCount != 2 {
		t.Fatalf("distinct sources: %d", c.SourceCount)
	}
	// 按首报时间序: 机器之心 (base) before 量子位 (base+2h).
	if len(c.SourceNames) != 2 || c.SourceNames[0] != "机器之心" || c.SourceNames[1] != "量子位" {
		t.Fatalf("source order: %+v", c.SourceNames)
	}
	if !c.FirstAt.Equal(base) || !c.LatestAt.Equal(base.Add(3*time.Hour)) {
		t.Fatalf("first/latest: %+v", c)
	}
	if c.Heat != 2 { // latestAt == now → no decay; sourceCount 2
		t.Fatalf("heat: %v", c.Heat)
	}
}
