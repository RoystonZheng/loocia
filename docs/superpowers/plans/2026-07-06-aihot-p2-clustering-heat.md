# P2 Clustering + Heat + Hot-Topics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build 事件聚类⑤ + 热度⑥ + the `/api/public/hot-topics` endpoint + the front-end 「当前热点」区: a periodic pass that groups same-event items via one LLM call, picks a primary per cluster, computes multi-source heat with exponential decay, and serves/renders the current hot topics.

**Architecture:** The internal proxy has NO embeddings endpoint (Anthropic Messages format), so clustering is **LLM batch grouping**: one `Complete` call over the window's items (id+title+source, ≤200) returns same-event groups as JSON; any parse/validation failure aborts the pass with **zero writes**. Primary = highest score, tie → earliest report (首报); `cluster_id` = primary's item id. Contract nuance honored: `duplicateOfId` (完全重复, hidden in both modes — untouched by this pass) is distinct from cluster **secondary** (hidden only in `selected`), so `items` gains a nullable `cluster_primary` column and the selected-mode query additionally requires `cluster_id IS NULL OR cluster_primary`. Heat = `sourceCount × 0.5^(hours_since_latest/24)`. The `clusters` table holds ONLY the current window and is fully rebuilt each pass (matches 「当前热点」 semantics). `cmd/hotpass` is the manual trigger (cron wiring stays a deployment task). The endpoint mirrors openapi `HotTopicList` (strips clusterId/heat) with a weak `W/"hot-…"` ETag + 304. The front-end shows the strip above the feed (in `App`, not inside `Feed`, so existing Feed tests stay untouched).

**Tech Stack:** Go (`internal/items` migration, new `internal/cluster`, `internal/publicapi` addition, `cmd/hotpass`), PostgreSQL, existing `internal/llm`; React/vitest.

---

## Environment / how to run

- Go via `server/Makefile` (`GOTOOLCHAIN=go1.25.5`; `make test` uses `-p 1` — DB suites share live tables). Single package: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org [envs] go test ./internal/<pkg>/ -run <name> -v`. **When running multiple DB-backed packages at once, always pass `-p 1`.**
- Live PG via SSH tunnel on `localhost:5432` (verify `nc -z localhost 5432`; restart `ssh -N -L 5432:localhost:5432 melos &`). DSN `postgres://aihot:aihot@localhost:5432/aihot_test`.
- LLM key (SECRET, never print/commit): `export AIHOT_LLM_API_KEY=$(ssh melos 'grep ^LLM_API_KEY /root/wechat-push/.env | cut -d= -f2-' | tr -d '"'\'' \r')`. Real-LLM smokes are optional/gated.
- Front-end: `cd web && npx vitest run` / `npm run build` (hermetic).

## Contract recap (openapi `/api/public/hot-topics`)

`HotTopicList {count, items:[HotTopic]}`; `HotTopic {id, title, url, permalink, source, sourceCount, sourceNames[] (按首报时间序), latestAt}` — 剥 clusterId / heat 数值. Sorted by multi-source heat (最热 first). Weak ETag (`W/"hot-…"`) + 304 + Cache-Control. Items-side filters: `duplicateOfId` 非空 → hidden both modes (already enforced); selected 额外 → cluster secondary hidden (THIS plan adds it).

## File Structure

- `server/internal/items/schema.sql` + `item.go` + `write.go` + `query.go` — `cluster_primary` column (ALTER … IF NOT EXISTS migration), 20-column lockstep, selected-mode secondary exclusion, `ClearClusters`/`AssignCluster`.
- `server/internal/cluster/cluster.go` — `Cluster` model, `choosePrimary`, `buildCluster`, `heatOf`.
- `server/internal/cluster/group.go` — `LLM` iface, `llmGroup` (prompt + strict parse).
- `server/internal/cluster/store.go` + `schema.sql` — clusters table, `ReplaceAll`, `ListHotTopics` (join primary item).
- `server/internal/cluster/pass.go` — `Pass.Run` orchestration.
- `server/cmd/hotpass/main.go` — manual trigger.
- `server/internal/publicapi/hot_handlers.go` — endpoint + ETag/304.
- `server/common/server/httpserv/httpserv.go` — mount.
- `web/src/api/hot.ts`, `web/src/components/HotTopics.tsx`, `web/src/App.tsx` — front-end strip.

---

### Task 1: items — `cluster_primary` + secondary exclusion + assignment writes

**Files:**
- Modify: `server/internal/items/schema.sql`, `item.go`, `write.go`, `query.go`
- Modify: `server/internal/items/write_test.go`, `query_test.go` (append tests)

- [ ] **Step 1: Write the failing tests**

Append to `server/internal/items/query_test.go`:
```go
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
```

Append to `server/internal/items/write_test.go`:
```go
func TestUpsertRoundTripsClusterPrimary(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pub := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	in := sampleItem("cp1", pub)
	cp := true
	cid := "cp1"
	in.ClusterID = &cid
	in.ClusterPrimary = &cp
	if err := s.Upsert(ctx, in); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.GetByID(ctx, "cp1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ClusterPrimary == nil || !*got.ClusterPrimary || got.ClusterID == nil || *got.ClusterID != "cp1" {
		t.Fatalf("cluster fields round-trip: %+v", got)
	}
}
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org AIHOT_TEST_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' go test ./internal/items/ -run 'TestSelectedModeHides|TestClearClusters|TestUpsertRoundTripsClusterPrimary' -v` → FAIL (`unknown field ClusterPrimary` / `undefined: AssignCluster`).

- [ ] **Step 3: Implement (20-column lockstep — keep all four sites in sync)**

`schema.sql`: append after the CREATE TABLE (before the indexes):
```sql
ALTER TABLE items ADD COLUMN IF NOT EXISTS cluster_primary BOOLEAN;
```
Also add `cluster_primary  BOOLEAN,` to the CREATE TABLE column list (after `duplicate_of_id`), so fresh databases match migrated ones.

`item.go`: add to the struct after `DuplicateOfID`:
```go
	ClusterPrimary *bool
```

`write.go`:
- `itemColumns` gains `, cluster_primary` at the END (20th column).
- `Upsert` VALUES gains `,$20`; args gain `, it.ClusterPrimary` at the end; ON CONFLICT SET gains `cluster_primary=EXCLUDED.cluster_primary,` (keep `updated_at=now()` last).
- `scanItem` gains `, &it.ClusterPrimary` at the end.
- Add the two write methods:
```go
// ClearClusters removes cluster assignments for items published in [since, until).
func (s *Store) ClearClusters(ctx context.Context, since, until time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE items SET cluster_id = NULL, cluster_primary = NULL
		WHERE published_at >= $1 AND published_at < $2`, since, until)
	return err
}

// AssignCluster marks one item as a member (primary or secondary) of a cluster.
func (s *Store) AssignCluster(ctx context.Context, id, clusterID string, primary bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE items SET cluster_id = $2, cluster_primary = $3 WHERE id = $1`, id, clusterID, primary)
	return err
}
```
(`write.go` needs the `time` import for the new signatures.)

`query.go`: in the List WHERE clause, add directly after the `selected = $1` line:
```
		  AND ($1::boolean IS NOT TRUE OR cluster_id IS NULL OR cluster_primary IS TRUE)
```
(Selected nil/false → no constraint; Selected true → only unclustered items or cluster primaries. No new parameter — reuses `$1`.)

- [ ] **Step 4: Run to confirm PASS** — same command → PASS (all three). Then the WHOLE items package `-run . -count=1` → all PASS (esp. the existing Upsert/keyset tests — the 20-column lockstep must not break them).

- [ ] **Step 5: Commit**
```bash
git add server/internal/items/
git commit -m "feat(items): cluster_primary + selected-mode secondary exclusion + cluster writes"
```

---

### Task 2: cluster store (clusters table + hot-topic join)

**Files:**
- Create: `server/internal/cluster/cluster.go`
- Create: `server/internal/cluster/schema.sql`
- Create: `server/internal/cluster/store.go`
- Create: `server/internal/cluster/helpers_test.go`
- Test: `server/internal/cluster/store_test.go`

- [ ] **Step 1: Write models + schema + store**

`server/internal/cluster/cluster.go`:
```go
package cluster

import (
	"math"
	"time"
)

// Cluster is one same-event group over the current window.
type Cluster struct {
	ID            string   // = primary item id
	PrimaryItemID string
	SourceCount   int      // distinct sources among members
	SourceNames   []string // 按首报时间序 (earliest report first)
	FirstAt       time.Time
	LatestAt      time.Time
	Heat          float64
}

// HotTopicRow is a cluster joined with its primary item's public fields,
// ordered by heat — the raw material for the /api/public/hot-topics endpoint.
type HotTopicRow struct {
	ID          string   // primary item id
	Title       string
	URL         string
	Permalink   string
	Source      string
	SourceCount int
	SourceNames []string
	LatestAt    time.Time
}

// heatOf: sourceCount weighted by exponential decay — halves every 24h since
// the latest report. Deterministic (now injected).
func heatOf(sourceCount int, latestAt, now time.Time) float64 {
	return float64(sourceCount) * math.Pow(0.5, now.Sub(latestAt).Hours()/24)
}
```

`server/internal/cluster/schema.sql`:
```sql
CREATE TABLE IF NOT EXISTS clusters (
    id              TEXT PRIMARY KEY,
    primary_item_id TEXT NOT NULL,
    source_count    INTEGER NOT NULL,
    source_names    JSONB NOT NULL,
    first_at        TIMESTAMPTZ NOT NULL,
    latest_at       TIMESTAMPTZ NOT NULL,
    heat            DOUBLE PRECISION NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`server/internal/cluster/store.go`:
```go
package cluster

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// Store persists the current window's clusters (fully rebuilt each pass).
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	return err
}

// ReplaceAll atomically swaps the clusters table content for the new pass.
func (s *Store) ReplaceAll(ctx context.Context, clusters []Cluster) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM clusters`); err != nil {
		return err
	}
	for _, c := range clusters {
		names, err := json.Marshal(c.SourceNames)
		if err != nil {
			return fmt.Errorf("marshal source_names: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO clusters (id, primary_item_id, source_count, source_names, first_at, latest_at, heat)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			c.ID, c.PrimaryItemID, c.SourceCount, names, c.FirstAt, c.LatestAt, c.Heat); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListHotTopics returns clusters heat-desc joined with their primary item's
// public fields. Internal heat is used for ORDER BY only (not exposed).
func (s *Store) ListHotTopics(ctx context.Context) ([]HotTopicRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.title, i.url, i.permalink, i.source,
		       c.source_count, c.source_names, c.latest_at
		FROM clusters c
		JOIN items i ON i.id = c.primary_item_id
		ORDER BY c.heat DESC, c.latest_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HotTopicRow
	for rows.Next() {
		var r HotTopicRow
		var names []byte
		if err := rows.Scan(&r.ID, &r.Title, &r.URL, &r.Permalink, &r.Source,
			&r.SourceCount, &names, &r.LatestAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(names, &r.SourceNames); err != nil {
			return nil, fmt.Errorf("unmarshal source_names: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 2: Write the tests**

`server/internal/cluster/helpers_test.go`:
```go
package cluster

import (
	"context"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
	"aihot-server/internal/items"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newLiveStores gives a cluster store + items store on a shared pool with
// clean clusters/items tables. Skips without the DSN.
func newLiveStores(t *testing.T) (*Store, *items.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("AIHOT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIHOT_TEST_DATABASE_URL not set")
	}
	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	t.Cleanup(pool.Close)
	cs := NewStore(pool)
	if err := cs.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("cluster EnsureSchema: %v", err)
	}
	is := items.New(pool)
	if err := is.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("items EnsureSchema: %v", err)
	}
	for _, tbl := range []string{"clusters", "items"} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+tbl); err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
	return cs, is, pool
}

func seedItem(t *testing.T, is *items.Store, id, source string, score int, pub time.Time, selected bool) items.Item {
	t.Helper()
	summary := "摘要-" + id
	cat := items.CategoryAIModels
	sc := score
	it := items.Item{
		ID: id, Title: "标题-" + id, URL: "https://x/" + id, Permalink: "/items/" + id,
		Source: source, PublishedAt: &pub, Summary: &summary, Category: &cat, Score: &sc,
		Selected: selected, Present: true,
	}
	if err := is.Upsert(context.Background(), it); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return it
}
```

`server/internal/cluster/store_test.go`:
```go
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

	seedItem(t, is, "hot1", "OpenAI Blog", 90, base, true)
	seedItem(t, is, "warm1", "机器之心", 70, base.Add(time.Hour), true)

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
```

- [ ] **Step 3: Run** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org AIHOT_TEST_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' go test ./internal/cluster/ -v` → PASS (2). Without DSN → live one SKIPs, `TestHeatOfDecay` still PASSes (pure).

- [ ] **Step 4: Build + commit**
```bash
cd server && make build
git add server/internal/cluster/
git commit -m "feat(cluster): clusters store + heat formula"
```

---

### Task 3: LLM grouping + primary choice (pure logic)

**Files:**
- Create: `server/internal/cluster/group.go`
- Test: `server/internal/cluster/group_test.go`

- [ ] **Step 1: Write the failing test**

`server/internal/cluster/group_test.go`:
```go
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
	its := []items.Item{mkIt("a", "S1", 90, base), mkIt("b", "S2", 80, base.Add(time.Hour)), mkIt("c", "S3", 70, base)}
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
	its := []items.Item{mkIt("a", "S1", 90, base), mkIt("b", "S2", 80, base)}

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
	p := choosePrimary([]items.Item{mkIt("a", "S1", 70, base), mkIt("b", "S2", 90, base.Add(time.Hour))})
	if p.ID != "b" {
		t.Fatalf("primary by score: %s", p.ID)
	}
	// Tie on score → earliest report (首报) wins.
	p = choosePrimary([]items.Item{mkIt("late", "S1", 80, base.Add(2*time.Hour)), mkIt("early", "S2", 80, base)})
	if p.ID != "early" {
		t.Fatalf("primary by 首报: %s", p.ID)
	}
}

func TestBuildClusterAggregates(t *testing.T) {
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)
	members := []items.Item{
		mkIt("m2", "量子位", 80, base.Add(2*time.Hour)),
		mkIt("m1", "机器之心", 90, base),
		mkIt("m3", "机器之心", 60, base.Add(3*time.Hour)), // duplicate source
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
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/cluster/ -run 'TestLLMGroup|TestChoosePrimary|TestBuildCluster' -v` → FAIL (`undefined: llmGroup`).

- [ ] **Step 3: Implement**

`server/internal/cluster/group.go`:
```go
package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
```
(`group.go` needs the `time` import for `buildCluster`.)

- [ ] **Step 4: Run to confirm PASS** — same command → PASS (all four functions).

- [ ] **Step 5: Commit**
```bash
git add server/internal/cluster/group.go server/internal/cluster/group_test.go
git commit -m "feat(cluster): LLM grouping + primary choice + cluster aggregation"
```

---

### Task 4: Pass orchestration + cmd/hotpass

**Files:**
- Create: `server/internal/cluster/pass.go`
- Test: `server/internal/cluster/pass_test.go`
- Create: `server/cmd/hotpass/main.go`

- [ ] **Step 1: Write the failing test**

`server/internal/cluster/pass_test.go`:
```go
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
	seedItem(t, is, "ev1a", "OpenAI Blog", 90, base, true)
	seedItem(t, is, "ev1b", "机器之心", 75, base.Add(time.Hour), true)
	seedItem(t, is, "solo", "量子位", 60, base.Add(2*time.Hour), true)

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
	seedItem(t, is, "x1", "S1", 80, now.Add(-time.Hour), true)
	seedItem(t, is, "x2", "S2", 70, now.Add(-2*time.Hour), true)

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
	seedItem(t, is, "old1", "S1", 80, now.Add(-time.Hour), true)
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
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org AIHOT_TEST_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' go test ./internal/cluster/ -run TestRun -v` → FAIL (`undefined: NewPass`).

- [ ] **Step 3: Implement**

`server/internal/cluster/pass.go`:
```go
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
	res.Clusters = len(clusters)
	return res, nil
}
```

`server/cmd/hotpass/main.go`:
```go
// hotpass runs one clustering + heat pass over the trailing window.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... \
//	  go run ./cmd/hotpass [-window-hours 72]
//
// The LLM key is REQUIRED — grouping is the LLM's job.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"aihot-server/internal/cluster"
	"aihot-server/internal/db"
	"aihot-server/internal/items"
	"aihot-server/internal/llm"
)

func main() {
	windowHours := flag.Int("window-hours", 72, "trailing window size in hours")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	client, err := llm.NewClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	cs := cluster.NewStore(pool)
	if err := cs.EnsureSchema(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}

	p := cluster.NewPass(items.New(pool), cs, client)
	res, err := p.Run(ctx, time.Duration(*windowHours)*time.Hour, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
	fmt.Printf("hotpass: %d window items → %d clusters\n", res.WindowItems, res.Clusters)
}
```

- [ ] **Step 4: Run to confirm PASS** — `... go test ./internal/cluster/ -v` → all PASS (store 2 + group 4 + pass 3). `make build` → green (compiles cmd/hotpass).

- [ ] **Step 5: Optional gated real-LLM smoke** — with key + DB: `cd server && AIHOT_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go run ./cmd/hotpass` → summary line (the DB's demo items may or may not group — report the line either way).

- [ ] **Step 6: Commit**
```bash
git add server/internal/cluster/pass.go server/internal/cluster/pass_test.go server/cmd/hotpass/
git commit -m "feat(cluster): pass orchestration + hotpass command"
```

---

### Task 5: hot-topics endpoint + mount

**Files:**
- Create: `server/internal/publicapi/hot_handlers.go`
- Test: `server/internal/publicapi/hot_handlers_test.go`
- Modify: `server/common/server/httpserv/httpserv.go`

- [ ] **Step 1: Write the failing test**

`server/internal/publicapi/hot_handlers_test.go`:
```go
package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aihot-server/internal/cluster"
)

type fakeHotStore struct {
	rows []cluster.HotTopicRow
	err  error
}

func (f *fakeHotStore) ListHotTopics(ctx context.Context) ([]cluster.HotTopicRow, error) {
	return f.rows, f.err
}

func TestHotTopicsEnvelopeAndShape(t *testing.T) {
	latest := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	f := &fakeHotStore{rows: []cluster.HotTopicRow{{
		ID: "h1", Title: "热点标题", URL: "https://x/h1", Permalink: "/items/h1",
		Source: "OpenAI Blog", SourceCount: 4, SourceNames: []string{"OpenAI Blog", "机器之心"},
		LatestAt: latest,
	}}}
	h := NewHotTopicsHandler(f)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var env struct {
		Count int `json:"count"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 1 || len(env.Items) != 1 {
		t.Fatalf("envelope: %+v", env)
	}
	it := env.Items[0]
	for _, k := range []string{"id", "title", "url", "permalink", "source", "sourceCount", "sourceNames", "latestAt"} {
		if _, ok := it[k]; !ok {
			t.Fatalf("missing key %s: %v", k, it)
		}
	}
	// Internal fields must NOT leak.
	for _, banned := range []string{"heat", "clusterId", "cluster_id", "primaryItemId"} {
		if _, ok := it[banned]; ok {
			t.Fatalf("internal field %s leaked: %v", banned, it)
		}
	}
	if it["sourceCount"].(float64) != 4 {
		t.Fatalf("sourceCount: %v", it["sourceCount"])
	}
}

func TestHotTopicsEmptyIsArray(t *testing.T) {
	h := NewHotTopicsHandler(&fakeHotStore{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil))
	body := rr.Body.String()
	if !contains(body, `"items":[]`) {
		t.Fatalf("empty items must be [] not null: %s", body)
	}
}

func TestHotTopicsETagAnd304(t *testing.T) {
	f := &fakeHotStore{rows: []cluster.HotTopicRow{{ID: "h1", Title: "t", URL: "u", Permalink: "/items/h1",
		Source: "S", SourceCount: 2, SourceNames: []string{"S"}, LatestAt: time.Now().UTC()}}}
	h := NewHotTopicsHandler(f)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil))
	etag := rr.Header().Get("ETag")
	if etag == "" || etag[:6] != `W/"hot` {
		t.Fatalf("weak hot ETag expected, got %q", etag)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil)
	req.Header.Set("If-None-Match", etag)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusNotModified || rr2.Body.Len() != 0 {
		t.Fatalf("want empty 304, got %d (%d bytes)", rr2.Code, rr2.Body.Len())
	}
}

func TestHotTopicsStoreErrorIs500(t *testing.T) {
	h := NewHotTopicsHandler(&fakeHotStore{err: context.DeadlineExceeded})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/hot-topics", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rr.Code)
	}
}
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/publicapi/ -run TestHotTopics -v` → FAIL (`undefined: NewHotTopicsHandler`).

- [ ] **Step 3: Implement**

`server/internal/publicapi/hot_handlers.go`:
```go
package publicapi

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aihot-server/internal/cluster"
)

// HotStore is the read surface the hot-topics handler needs (satisfied by *cluster.Store).
type HotStore interface {
	ListHotTopics(ctx context.Context) ([]cluster.HotTopicRow, error)
}

// hotTopic mirrors openapi HotTopic — internal clusterId/heat deliberately absent.
type hotTopic struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Permalink   string    `json:"permalink"`
	Source      string    `json:"source"`
	SourceCount int       `json:"sourceCount"`
	SourceNames []string  `json:"sourceNames"`
	LatestAt    time.Time `json:"latestAt"`
}

type hotTopicList struct {
	Count int        `json:"count"`
	Items []hotTopic `json:"items"`
}

// NewHotTopicsHandler serves GET /api/public/hot-topics.
func NewHotTopicsHandler(s HotStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.ListHotTopics(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		env := hotTopicList{Count: len(rows), Items: make([]hotTopic, 0, len(rows))}
		for _, row := range rows {
			env.Items = append(env.Items, hotTopic{
				ID: row.ID, Title: row.Title, URL: row.URL, Permalink: row.Permalink,
				Source: row.Source, SourceCount: row.SourceCount, SourceNames: row.SourceNames,
				LatestAt: row.LatestAt,
			})
		}
		body, err := json.Marshal(env)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encoding error")
			return
		}
		sum := sha1.Sum(body)
		etag := fmt.Sprintf(`W/"hot-%x"`, sum[:8])
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "public, s-maxage=300, stale-while-revalidate=300")
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}
```

- [ ] **Step 4: Run to confirm PASS** — same command → PASS (all four). Whole publicapi `-run . -count=1` → all PASS.

- [ ] **Step 5: Mount.** In `httpserv.go`, next to the daily mounts, add (READ the file first; follow the existing pattern):
```go
hotStore := cluster.NewStore(pool)
if pool != nil {
	if err := hotStore.EnsureSchema(context.Background()); err != nil {
		fmt.Printf("[aihot] cluster EnsureSchema failed: %v\n", err)
	}
}
svr.AddHTTPHandle("/api/public/hot-topics", publicapi.NewHotTopicsHandler(hotStore))
```

- [ ] **Step 6: Live boot smoke** — `cd server && AIHOT_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' make run &` → wait for :8991, then `curl -si localhost:8991/api/public/hot-topics | head -6` → 200 with ETag `W/"hot-…"` and a JSON envelope (`items` may be empty — fine). Kill the server after.

- [ ] **Step 7: Commit**
```bash
git add server/internal/publicapi/hot_handlers.go server/internal/publicapi/hot_handlers_test.go server/common/server/httpserv/httpserv.go
git commit -m "feat(publicapi): hot-topics endpoint + mount"
```

---

### Task 6: Front-end 「当前热点」区

**Files:**
- Create: `web/src/api/hot.ts`
- Test: `web/src/api/hot.test.ts`
- Create: `web/src/components/HotTopics.tsx`
- Test: `web/src/components/HotTopics.test.tsx`
- Modify: `web/src/App.tsx`, `web/src/App.test.tsx`, `web/src/index.css`

- [ ] **Step 1: Client + test (TDD)**

`web/src/api/hot.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest'
import { fetchHotTopics } from './hot'

describe('fetchHotTopics', () => {
  it('returns the list', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ count: 1, items: [{ id: 'h1', title: '热点', url: 'https://x/h1', permalink: '/items/h1', source: 'S', sourceCount: 3, sourceNames: ['S'], latestAt: '2026-05-07T10:00:00Z' }] }),
    }) as unknown as typeof fetch
    const out = await fetchHotTopics()
    expect(out.count).toBe(1)
    expect(out.items[0].sourceCount).toBe(3)
  })

  it('throws on non-ok', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    await expect(fetchHotTopics()).rejects.toThrow(/500/)
  })
})
```

`web/src/api/hot.ts`:
```ts
export interface HotTopic {
  id: string
  title: string
  url: string
  permalink: string
  source: string
  sourceCount: number
  sourceNames: string[]
  latestAt: string
}

export interface HotTopicList {
  count: number
  items: HotTopic[]
}

export async function fetchHotTopics(): Promise<HotTopicList> {
  const res = await fetch('/api/public/hot-topics')
  if (!res.ok) throw new Error(`hot-topics fetch failed: ${res.status}`)
  return (await res.json()) as HotTopicList
}
```

- [ ] **Step 2: HotTopics component + test (TDD)**

`web/src/components/HotTopics.test.tsx`:
```tsx
import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { HotTopics } from './HotTopics'

describe('HotTopics', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders topics with source-count badges', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ count: 2, items: [
        { id: 'h1', title: '大新闻', url: 'https://x/h1', permalink: '/items/h1', source: 'A', sourceCount: 4, sourceNames: ['A', 'B'], latestAt: '2026-05-07T10:00:00Z' },
        { id: 'h2', title: '次热', url: 'https://x/h2', permalink: '/items/h2', source: 'C', sourceCount: 2, sourceNames: ['C'], latestAt: '2026-05-07T09:00:00Z' },
      ] }),
    }) as unknown as typeof fetch
    render(<HotTopics />)
    await waitFor(() => expect(screen.getByText('当前热点')).toBeInTheDocument())
    expect(screen.getByRole('link', { name: '大新闻' })).toHaveAttribute('href', '/items/h1')
    expect(screen.getByText('4 个来源')).toBeInTheDocument()
    expect(screen.getByText('次热')).toBeInTheDocument()
  })

  it('renders nothing when empty', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ count: 0, items: [] }) }) as unknown as typeof fetch
    const { container } = render(<HotTopics />)
    await waitFor(() => expect(container.innerHTML).toBe(''))
  })

  it('renders nothing on error', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    const { container } = render(<HotTopics />)
    await waitFor(() => expect(container.innerHTML).toBe(''))
  })
})
```

`web/src/components/HotTopics.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { fetchHotTopics, type HotTopic } from '../api/hot'

// HotTopics renders the 「当前热点」 strip; it disappears entirely when there
// are no topics or the fetch fails (the feed must not depend on it).
export function HotTopics() {
  const [topics, setTopics] = useState<HotTopic[]>([])

  useEffect(() => {
    fetchHotTopics()
      .then((res) => setTopics(res.items))
      .catch(() => setTopics([]))
  }, [])

  if (topics.length === 0) return null

  return (
    <section className="hot-topics">
      <h2 className="hot-title">当前热点</h2>
      {topics.map((t) => (
        <div key={t.id} className="hot-row">
          <a className="hot-link" href={t.permalink}>{t.title}</a>
          <span className="hot-badge">{t.sourceCount} 个来源</span>
        </div>
      ))}
    </section>
  )
}
```

- [ ] **Step 3: App integration + test.** In `web/src/App.test.tsx`, extend the URL-routed fetch mock with a `hot-topics` branch returning `{ count: 1, items: [{ id: 'h1', title: '热点一', url: 'https://x/h1', permalink: '/items/h1', source: 'S', sourceCount: 3, sourceNames: ['S'], latestAt: '2026-05-07T10:00:00Z' }] }` (add the branch BEFORE the items fallback), and add to the existing test after the feed assertion:
```tsx
  // hot-topics strip renders above the feed
  await waitFor(() => expect(screen.getByText('当前热点')).toBeInTheDocument())
  expect(screen.getByText('热点一')).toBeInTheDocument()
```
In `web/src/App.tsx`, render the strip above the feed (feed view only):
```tsx
      <main>{view === 'feed' ? (<><HotTopics /><Feed /></>) : <DailyView />}</main>
```
(plus the `import { HotTopics } from './components/HotTopics'`.)

Append to `web/src/index.css`:
```css
.hot-topics { border: 1px solid #f0d9a8; background: #fffaf0; border-radius: 8px; padding: 12px 16px; margin-bottom: 16px; }
.hot-title { margin: 0 0 8px; font-size: 1rem; color: #9a6b00; }
.hot-row { display: flex; justify-content: space-between; align-items: baseline; gap: 10px; margin-bottom: 6px; }
.hot-link { text-decoration: none; color: #111; font-weight: 500; }
.hot-link:hover { text-decoration: underline; }
.hot-badge { flex: none; font-size: .75rem; color: #9a6b00; background: #f7ecd2; padding: 1px 8px; border-radius: 999px; }
@media (prefers-color-scheme: dark) {
  .hot-topics { background: #221c10; border-color: #4a3b1a; }
  .hot-link { color: #eee; }
  .hot-badge { background: #3a2f14; }
}
```

- [ ] **Step 4: Full suite + build** — `cd web && npx vitest run` → all PASS; `npm run build` → green.

- [ ] **Step 5: Commit**
```bash
git add web/src/api/hot.ts web/src/api/hot.test.ts web/src/components/HotTopics.tsx web/src/components/HotTopics.test.tsx web/src/App.tsx web/src/App.test.tsx web/src/index.css
git commit -m "feat(web): 当前热点 strip"
```

---

## Self-Review

- **Spec coverage:** Implements 聚类⑤ (LLM batch grouping — the design doc's embedding approach is unavailable on the Anthropic-format proxy, an explicit substitution; duplicateOfId semantics untouched), 热度⑥ (`sourceCount × 0.5^(h/24)`, recomputed each pass), the openapi hot-topics contract (shape, 按首报时间序 sourceNames, heat/clusterId stripped, weak `W/"hot-…"` ETag + 304), the selected-mode secondary exclusion (openapi 隐式过滤 for selected), `cmd/hotpass`, and the 前端热点区 (置顶精选页, hidden when empty/error). Deliberately deferred (with a home): cron scheduling of hotpass (deployment wiring), cross-window duplicate flagging via duplicateOfId (needs a policy for 完全重复 detection — LLM grouping treats same-event ≠ identical), pagination/take on hot-topics (contract has no params), and tuning of the 72h window / decay constant (operational knobs).
- **Placeholder scan:** No TBD/TODO. Every step has complete code or an exact command. Task 5's mount step points at the existing httpserv.go pattern (implementer reads it) — honest, consistent with prior plans.
- **Type consistency:** `items.Item.ClusterPrimary *bool` ↔ 20-column lockstep (itemColumns/VALUES/$20/ON CONFLICT/scanItem — Step 3 lists all four sites); `ClearClusters(since, until)`/`AssignCluster(id, clusterID, primary)` used by `Pass.Run`; `cluster.Cluster`/`HotTopicRow`/`heatOf`/`choosePrimary`/`buildCluster`/`llmGroup`/`LLM` defined once; `Store.ReplaceAll`/`ListHotTopics` match the `publicapi.HotStore` interface; `fakeLLM` defined in group_test.go and reused by pass_test.go (same package); front-end `HotTopic`/`HotTopicList` mirror the wire shape. `buildCluster`'s `now` injection keeps heat deterministic in tests.

## Follow-on

- **Deployment wiring**: cron hotpass (e.g. every 30min) + ingest/pipeline/gendaily on a VPN host.
- **完全重复 detection** (duplicateOfId assignment) — a separate policy/task.
- **Window/decay tuning** once real volume exists; optionally persist heat history.
- **/items/{id} detail page** — hot-topic links point at permalinks (same 404 as feed/daily today).
