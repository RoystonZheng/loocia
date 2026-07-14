# 交互式词云图谱 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增「图谱」主视图：LLM 从每条资讯抽实体+话题词入 `item_terms` 表，词云（d3-cloud）看热度，点词开共现子图（d3-force）+ 相关资讯。

**Architecture:** 后端照既有模式扩三层——terms store（新表+SQL 聚合）、pipeline 抽取步骤（best-effort，低档模型）、publicapi 两个只读端点（实时 SQL，无预计算）。前端新增 GraphView 视图，d3-cloud/d3-force 只做布局计算，渲染全部自绘 SVG。

**Tech Stack:** Go 1.25 + pgx v5；React 19 + TS + Vite；npm 新依赖仅 `d3-cloud` `d3-force`（+ @types）。

**Spec:** `docs/superpowers/specs/2026-07-14-knowledge-graph-design.md`

**铁律（每个任务都适用）:**
- Go 命令在 `server/` 下跑，前缀 `GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org`。
- DB 测试没有 `AIHOT_TEST_DATABASE_URL` 会自动 skip，属正常。
- 提交署名：`git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit`，直接提交 `main`。
- 提交前该任务涉及的 Go 文件 `gofmt -l` 必须干净。
- 前端测试用 `fireEvent`，不用 user-event。

## File Structure

**后端（Create）：**
- `server/internal/terms/schema.sql` — item_terms 表
- `server/internal/terms/store.go` — Store：ReplaceForItem / Cloud / Neighbors / ItemsForTerm / TermInfo
- `server/internal/terms/store_test.go`
- `server/internal/pipeline/terms.go` — TermExtractor（LLM 抽取 + 解析归一）
- `server/internal/pipeline/terms_test.go`
- `server/internal/publicapi/graph_handlers.go` — /graph/cloud、/graph/term/{term}
- `server/internal/publicapi/graph_handlers_test.go`
- `server/cmd/backfillterms/main.go` — 存量回填

**后端（Modify）：**
- `server/internal/llm/client.go` — NewTermsClientFromEnv
- `server/internal/pipeline/processor.go` — WithTermExtractor 挂载
- `server/internal/pulse/pulse.go` + `server/cmd/pulse/main.go` — 接线
- `server/common/server/httpserv/httpserv.go` — 注册两个路由
- `server/internal/items/helpers_test.go`、`server/internal/pipeline/helpers_test.go` — `TRUNCATE items` → `TRUNCATE items CASCADE`（新外键会让裸 TRUNCATE 报错）

**前端（Create）：**
- `web/src/api/graph.ts` + `graph.test.ts`
- `web/src/cloudLayout.ts` — d3-cloud 封装（jsdom 无 canvas，测试中 mock 此模块）
- `web/src/graphLayout.ts` + `graphLayout.test.ts` — d3-force 同步布局（纯数学，可直接测）
- `web/src/components/TermPanel.tsx` + `TermPanel.test.tsx`
- `web/src/components/GraphView.tsx` + `GraphView.test.tsx`

**前端（Modify）：**
- `web/src/components/Sidebar.tsx` — View 加 'graph' + 导航项
- `web/src/App.tsx` — graph 分支
- `web/src/App.test.tsx` — mockAll 加 graph 端点 + 切换测试
- `web/src/index.css` — 图谱视图样式 + 600px 断点

---

### Task 1: terms store（表 + 五个方法）

**Files:**
- Create: `server/internal/terms/schema.sql`, `server/internal/terms/store.go`
- Test: `server/internal/terms/store_test.go`
- Modify: `server/internal/items/helpers_test.go:27`, `server/internal/pipeline/helpers_test.go:41-44`

- [ ] **Step 1: 写 schema 和测试**

`server/internal/terms/schema.sql`：

```sql
CREATE TABLE IF NOT EXISTS item_terms (
    item_id TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    term    TEXT NOT NULL,
    kind    TEXT NOT NULL CHECK (kind IN ('entity','topic')),
    PRIMARY KEY (item_id, term)
);

CREATE INDEX IF NOT EXISTS item_terms_term_idx ON item_terms (term);
```

`server/internal/terms/store_test.go`（items 表是外键前置，用 items.Store 造数据；`TRUNCATE items CASCADE` 顺带清 item_terms）：

```go
package terms

import (
	"context"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
	"aihot-server/internal/items"
)

// newTestStores brings up items (FK prerequisite) + terms schemas on the test DB.
func newTestStores(t *testing.T) (*items.Store, *Store) {
	t.Helper()
	dsn := os.Getenv("AIHOT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIHOT_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	t.Cleanup(pool.Close)
	is := items.New(pool)
	if err := is.EnsureSchema(ctx); err != nil {
		t.Fatalf("items EnsureSchema: %v", err)
	}
	ts := NewStore(pool)
	if err := ts.EnsureSchema(ctx); err != nil {
		t.Fatalf("terms EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE items CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return is, ts
}

func seedItem(t *testing.T, is *items.Store, id string, published time.Time) {
	t.Helper()
	if err := is.Upsert(context.Background(), items.Item{
		ID: id, Title: "title-" + id, URL: "https://example.com/" + id,
		Permalink: "/items/" + id, Source: "Example", PublishedAt: &published, Present: true,
	}); err != nil {
		t.Fatalf("upsert %s: %v", id, err)
	}
}

func TestReplaceForItemIdempotent(t *testing.T) {
	is, ts := newTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedItem(t, is, "a", now)

	if err := ts.ReplaceForItem(ctx, "a", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "开源", Kind: "topic"}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	// 重跑换一套词：旧词必须被清掉
	if err := ts.ReplaceForItem(ctx, "a", []Term{{Term: "英伟达", Kind: "entity"}}); err != nil {
		t.Fatalf("replace2: %v", err)
	}
	cloud, err := ts.Cloud(ctx, nil, 10)
	if err != nil {
		t.Fatalf("cloud: %v", err)
	}
	if len(cloud) != 1 || cloud[0].Term != "英伟达" || cloud[0].Count != 1 {
		t.Fatalf("cloud after replace: %+v", cloud)
	}
}

func TestCloudCountsWindowAndKind(t *testing.T) {
	is, ts := newTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedItem(t, is, "new1", now.Add(-1*time.Hour))
	seedItem(t, is, "new2", now.Add(-2*time.Hour))
	seedItem(t, is, "old", now.Add(-40*24*time.Hour))

	must := func(id string, ts2 []Term) {
		if err := ts.ReplaceForItem(ctx, id, ts2); err != nil {
			t.Fatalf("replace %s: %v", id, err)
		}
	}
	must("new1", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "开源", Kind: "topic"}})
	must("new2", []Term{{Term: "OpenAI", Kind: "entity"}})
	must("old", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "远古话题", Kind: "topic"}})

	// all（since=nil）：OpenAI=3 居首
	all, err := ts.Cloud(ctx, nil, 10)
	if err != nil {
		t.Fatalf("cloud all: %v", err)
	}
	if len(all) != 3 || all[0].Term != "OpenAI" || all[0].Count != 3 || all[0].Kind != "entity" {
		t.Fatalf("cloud all: %+v", all)
	}
	// 7d 窗口：old 被滤掉
	since := now.Add(-7 * 24 * time.Hour)
	recent, err := ts.Cloud(ctx, &since, 10)
	if err != nil {
		t.Fatalf("cloud 7d: %v", err)
	}
	if len(recent) != 2 || recent[0].Count != 2 {
		t.Fatalf("cloud 7d: %+v", recent)
	}
	for _, c := range recent {
		if c.Term == "远古话题" {
			t.Fatalf("window leak: %+v", recent)
		}
	}
}

func TestNeighborsCooccurrenceAndClusterBoost(t *testing.T) {
	is, ts := newTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedItem(t, is, "plain", now.Add(-1*time.Hour))
	seedItem(t, is, "clustered", now.Add(-2*time.Hour))
	// clustered 属于一个多源事件 cluster → 该条里的共现权重 ×2
	if err := is.AssignCluster(ctx, "clustered", "clustered", true); err != nil {
		t.Fatalf("assign cluster: %v", err)
	}

	if err := ts.ReplaceForItem(ctx, "plain", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "开源", Kind: "topic"}}); err != nil {
		t.Fatalf("replace plain: %v", err)
	}
	if err := ts.ReplaceForItem(ctx, "clustered", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "推理模型", Kind: "topic"}}); err != nil {
		t.Fatalf("replace clustered: %v", err)
	}

	ns, err := ts.Neighbors(ctx, "OpenAI", nil, 10)
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	if len(ns) != 2 {
		t.Fatalf("neighbors: %+v", ns)
	}
	// 加权后 推理模型(2) 排在 开源(1) 前面
	if ns[0].Term != "推理模型" || ns[0].Weight != 2 || ns[1].Term != "开源" || ns[1].Weight != 1 {
		t.Fatalf("weights: %+v", ns)
	}
}

func TestItemsForTermAndTermInfo(t *testing.T) {
	is, ts := newTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedItem(t, is, "newer", now.Add(-1*time.Hour))
	seedItem(t, is, "older", now.Add(-3*time.Hour))
	for _, id := range []string{"newer", "older"} {
		if err := ts.ReplaceForItem(ctx, id, []Term{{Term: "OpenAI", Kind: "entity"}}); err != nil {
			t.Fatalf("replace %s: %v", id, err)
		}
	}

	its, err := ts.ItemsForTerm(ctx, "OpenAI", nil, 10)
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	if len(its) != 2 || its[0].ID != "newer" || its[1].ID != "older" {
		t.Fatalf("order: %+v", its)
	}
	if its[0].Title != "title-newer" || its[0].Permalink != "/items/newer" {
		t.Fatalf("fields: %+v", its[0])
	}

	kind, count, err := ts.TermInfo(ctx, "OpenAI", nil)
	if err != nil {
		t.Fatalf("terminfo: %v", err)
	}
	if kind != "entity" || count != 2 {
		t.Fatalf("terminfo: kind=%q count=%d", kind, count)
	}
	// 不存在的词：count=0 不报错
	kind, count, err = ts.TermInfo(ctx, "没有的词", nil)
	if err != nil || count != 0 || kind != "" {
		t.Fatalf("missing term: kind=%q count=%d err=%v", kind, count, err)
	}
}
```

- [ ] **Step 2: 跑测试确认编译失败**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/terms/
```
Expected: FAIL（package 不存在 / NewStore undefined）

- [ ] **Step 3: 实现 store**

`server/internal/terms/store.go`：

```go
// Package terms stores per-item extracted terms (entities + topics) and serves
// the aggregations behind the graph view: word-cloud counts, co-occurrence
// neighbors, and per-term item lists. All queries are real-time SQL — at the
// current scale (~1k items) precomputation would be waste.
package terms

import (
	"context"
	_ "embed"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// Term is one extracted term of an item.
type Term struct {
	Term string
	Kind string // 'entity' | 'topic'
}

// CloudTerm is one word-cloud entry: term + distinct-item count in the window.
type CloudTerm struct {
	Term  string
	Kind  string
	Count int
}

// Neighbor is one co-occurrence edge endpoint: items where both terms appear
// count 1 each, or 2 when the item belongs to a hot cluster (same-event boost).
type Neighbor struct {
	Term   string
	Kind   string
	Weight int
}

// TermItem is the public projection of an item carrying a term (the same
// pattern as cluster.HotTopicRow: a local row struct keeps packages decoupled).
type TermItem struct {
	ID          string
	Title       string
	TitleEN     *string
	URL         string
	Permalink   string
	Source      string
	PublishedAt *time.Time
	Summary     *string
	ImageURL    *string
	Category    *string
	Score       *int
	Selected    bool
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	return err
}

// ReplaceForItem swaps an item's terms atomically (delete + insert in one tx),
// so re-extraction never leaves stale rows behind.
func (s *Store) ReplaceForItem(ctx context.Context, itemID string, ts []Term) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM item_terms WHERE item_id = $1`, itemID); err != nil {
		return err
	}
	for _, t := range ts {
		if _, err := tx.Exec(ctx,
			`INSERT INTO item_terms (item_id, term, kind) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			itemID, t.Term, t.Kind); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Cloud returns the top terms by distinct-item count since `since` (nil = all
// time). MIN(kind) prefers 'entity' when the same term was tagged both ways.
func (s *Store) Cloud(ctx context.Context, since *time.Time, limit int) ([]CloudTerm, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.term, MIN(t.kind), COUNT(*)::int AS cnt
		FROM item_terms t
		JOIN items i ON i.id = t.item_id
		WHERE i.present
		  AND ($1::timestamptz IS NULL OR COALESCE(i.published_at,'epoch'::timestamptz) >= $1)
		GROUP BY t.term
		ORDER BY cnt DESC, t.term
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudTerm
	for rows.Next() {
		var c CloudTerm
		if err := rows.Scan(&c.Term, &c.Kind, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Neighbors returns terms co-occurring with `term` (same item), weight-desc.
// An item inside a hot cluster (same-event, multi-source) counts double.
func (s *Store) Neighbors(ctx context.Context, term string, since *time.Time, limit int) ([]Neighbor, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.term, MIN(b.kind),
		       SUM(CASE WHEN i.cluster_id IS NOT NULL THEN 2 ELSE 1 END)::int AS weight
		FROM item_terms a
		JOIN item_terms b ON b.item_id = a.item_id AND b.term <> a.term
		JOIN items i ON i.id = a.item_id
		WHERE a.term = $1 AND i.present
		  AND ($2::timestamptz IS NULL OR COALESCE(i.published_at,'epoch'::timestamptz) >= $2)
		GROUP BY b.term
		ORDER BY weight DESC, b.term
		LIMIT $3`, term, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Neighbor
	for rows.Next() {
		var n Neighbor
		if err := rows.Scan(&n.Term, &n.Kind, &n.Weight); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ItemsForTerm returns the newest items carrying the term, public fields only.
func (s *Store) ItemsForTerm(ctx context.Context, term string, since *time.Time, limit int) ([]TermItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.title, i.title_en, i.url, i.permalink, i.source, i.published_at,
		       i.summary, i.image_url, i.category, i.score, i.selected
		FROM item_terms t
		JOIN items i ON i.id = t.item_id
		WHERE t.term = $1 AND i.present
		  AND ($2::timestamptz IS NULL OR COALESCE(i.published_at,'epoch'::timestamptz) >= $2)
		ORDER BY COALESCE(i.published_at,'epoch'::timestamptz) DESC, i.id DESC
		LIMIT $3`, term, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TermItem
	for rows.Next() {
		var it TermItem
		if err := rows.Scan(&it.ID, &it.Title, &it.TitleEN, &it.URL, &it.Permalink, &it.Source,
			&it.PublishedAt, &it.Summary, &it.ImageURL, &it.Category, &it.Score, &it.Selected); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// TermInfo returns a term's kind + item count in the window; ("", 0, nil) when
// the term doesn't appear at all (the handler still answers 200 with empties).
func (s *Store) TermInfo(ctx context.Context, term string, since *time.Time) (kind string, count int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(MIN(t.kind), ''), COUNT(*)::int
		FROM item_terms t
		JOIN items i ON i.id = t.item_id
		WHERE t.term = $1 AND i.present
		  AND ($2::timestamptz IS NULL OR COALESCE(i.published_at,'epoch'::timestamptz) >= $2)`,
		term, since).Scan(&kind, &count)
	return kind, count, err
}
```

- [ ] **Step 4: 修两个既有 test helper 的 TRUNCATE**

`server/internal/items/helpers_test.go` 第 27 行：

```go
	if _, err := pool.Exec(context.Background(), "TRUNCATE items CASCADE"); err != nil {
```

`server/internal/pipeline/helpers_test.go` 第 43 行（raw_items 那句不动）：

```go
	if _, err := pool.Exec(ctx, "TRUNCATE items CASCADE"); err != nil {
```

- [ ] **Step 5: 跑测试确认通过**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/terms/ ./internal/items/ ./internal/pipeline/
```
Expected: PASS（无 test DB 时 terms/items 的 DB 测试 skip，同样算过）

- [ ] **Step 6: gofmt + commit**

```bash
cd server && gofmt -l internal/terms/ internal/items/ internal/pipeline/   # 必须无输出
git add internal/terms/ internal/items/helpers_test.go internal/pipeline/helpers_test.go
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(terms): item_terms 表 + 词云/共现/相关资讯聚合 store"
```

---

### Task 2: llm.NewTermsClientFromEnv

**Files:**
- Modify: `server/internal/llm/client.go:78`（NewTranslateClientFromEnv 之后）
- Test: `server/internal/llm/client_env_test.go`（Create）

- [ ] **Step 1: 写失败测试**

`server/internal/llm/client_env_test.go`：

```go
package llm

import "testing"

func TestNewTermsClientFromEnvDefaults(t *testing.T) {
	t.Setenv("AIHOT_LLM_API_KEY", "k")
	t.Setenv("AIHOT_TERMS_MODEL", "")
	c, err := NewTermsClientFromEnv()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c.model != "deepseek-v4-flash" {
		t.Fatalf("default model: %q", c.model)
	}

	t.Setenv("AIHOT_TERMS_MODEL", "auto-mini")
	c, err = NewTermsClientFromEnv()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c.model != "auto-mini" {
		t.Fatalf("override model: %q", c.model)
	}
}

func TestNewTermsClientFromEnvRequiresKey(t *testing.T) {
	t.Setenv("AIHOT_LLM_API_KEY", "")
	if _, err := NewTermsClientFromEnv(); err == nil {
		t.Fatal("expected error without AIHOT_LLM_API_KEY")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/llm/
```
Expected: FAIL（NewTermsClientFromEnv undefined）

- [ ] **Step 3: 实现**

`server/internal/llm/client.go`，加在 `NewTranslateClientFromEnv` 之后：

```go
// defaultTermsModel: term extraction is a short, structured task — the cheap
// tier is plenty and mustn't compete with auto-max for the shared key's RPM.
const defaultTermsModel = "deepseek-v4-flash"

// NewTermsClientFromEnv builds a client on the same proxy/key as
// NewClientFromEnv but with the lower-tier terms model (AIHOT_TERMS_MODEL
// override, default deepseek-v4-flash).
func NewTermsClientFromEnv() (*Client, error) {
	key := os.Getenv("AIHOT_LLM_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("AIHOT_LLM_API_KEY not set")
	}
	base := os.Getenv("AIHOT_LLM_BASE_URL")
	if base == "" {
		base = defaultBaseURL
	}
	model := os.Getenv("AIHOT_TERMS_MODEL")
	if model == "" {
		model = defaultTermsModel
	}
	return NewClient(base, key, model), nil
}
```

- [ ] **Step 4: 跑测试确认通过**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/llm/
```
Expected: PASS

- [ ] **Step 5: gofmt + commit**

```bash
cd server && gofmt -l internal/llm/
git add internal/llm/
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(llm): 低档 terms 抽取 client（AIHOT_TERMS_MODEL）"
```

---

### Task 3: pipeline TermExtractor（抽取 + 解析归一）

**Files:**
- Create: `server/internal/pipeline/terms.go`
- Test: `server/internal/pipeline/terms_test.go`

- [ ] **Step 1: 写失败测试**

`server/internal/pipeline/terms_test.go`（fakeLLM 直接内联，不依赖 DB）：

```go
package pipeline

import (
	"context"
	"reflect"
	"testing"

	"aihot-server/internal/terms"
)

type fakeTermsLLM struct{ out string }

func (f fakeTermsLLM) Complete(ctx context.Context, system, user string) (string, error) {
	return f.out, nil
}

func TestExtractParsesPlainJSON(t *testing.T) {
	x := NewTermExtractor(fakeTermsLLM{out: `{"entities":["OpenAI","GPT-5.5"],"topics":["推理模型"]}`})
	got, err := x.Extract(context.Background(), "标题", "摘要")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := Terms{Entities: []string{"OpenAI", "GPT-5.5"}, Topics: []string{"推理模型"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractStripsCodeFence(t *testing.T) {
	x := NewTermExtractor(fakeTermsLLM{out: "```json\n{\"entities\":[\"英伟达\"],\"topics\":[]}\n```"})
	got, err := x.Extract(context.Background(), "t", "s")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got.Entities) != 1 || got.Entities[0] != "英伟达" || len(got.Topics) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseTermsSanitizes(t *testing.T) {
	// 超限截断、空串/纯空白剔除、重复剔除、超长词剔除
	long := make([]rune, 41)
	for i := range long {
		long[i] = '长'
	}
	got, err := parseTerms(`{"entities":["A","B","C","D","E","F","","A"],"topics":["x","x","  ","` + string(long) + `","y","z","w"]}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(got.Entities, []string{"A", "B", "C", "D", "E"}) {
		t.Fatalf("entities: %+v", got.Entities)
	}
	if !reflect.DeepEqual(got.Topics, []string{"x", "y", "z"}) {
		t.Fatalf("topics: %+v", got.Topics)
	}
}

func TestParseTermsRejectsGarbage(t *testing.T) {
	if _, err := parseTerms("抱歉，我无法处理"); err == nil {
		t.Fatal("expected error on non-JSON output")
	}
}

func TestTermRowsEntityWinsDedupe(t *testing.T) {
	rows := termRows(Terms{Entities: []string{"OpenAI"}, Topics: []string{"OpenAI", "开源"}})
	want := []terms.Term{{Term: "OpenAI", Kind: "entity"}, {Term: "开源", Kind: "topic"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows: %+v", rows)
	}
}

func TestTermRowsEmpty(t *testing.T) {
	if rows := termRows(Terms{}); len(rows) != 0 {
		t.Fatalf("rows: %+v", rows)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -run 'Extract|ParseTerms|TermRows'
```
Expected: FAIL（NewTermExtractor undefined）

- [ ] **Step 3: 实现**

`server/internal/pipeline/terms.go`：

```go
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aihot-server/internal/terms"
)

// Terms is the structured term-extraction output for one item.
type Terms struct {
	Entities []string `json:"entities"`
	Topics   []string `json:"topics"`
}

// TermExtractor pulls entities + topic keywords out of an item's title+summary.
type TermExtractor interface {
	Extract(ctx context.Context, title, summary string) (Terms, error)
}

const termsSystemPrompt = `你是 AI 资讯的信息抽取器。给定一条资讯的标题和摘要，输出一个 JSON 对象（只输出 JSON，不要任何解释或代码块外的文字），字段：
- "entities": 数组，最多 5 个，资讯里出现的具体实体（公司/组织、模型/产品、人物）。命名要规范统一：模型与产品保留官方英文名（如 GPT-5.5、Claude、Gemini、DeepSeek-V4）；公司/组织用最常用的单一名称（如 OpenAI、谷歌、微软、英伟达、智谱），同一对象永远用同一个写法；人物用全名。
- "topics": 数组，最多 3 个，抽象话题词，每个 2~6 个字的中文（如 推理模型、开源、具身智能、视频生成、芯片、评测）。宁缺毋滥，不要生造。
没有可抽的就给空数组。`

const (
	maxEntities  = 5
	maxTopics    = 3
	maxTermRunes = 40
)

type termExtractor struct{ llm LLM }

// NewTermExtractor wraps an LLM (typically the low-tier terms client).
func NewTermExtractor(llm LLM) TermExtractor { return &termExtractor{llm: llm} }

func (t *termExtractor) Extract(ctx context.Context, title, summary string) (Terms, error) {
	user := "标题：" + title + "\n摘要：" + summary
	out, err := t.llm.Complete(ctx, termsSystemPrompt, user)
	if err != nil {
		return Terms{}, err
	}
	return parseTerms(out)
}

// parseTerms extracts the JSON object and sanitizes both lists: trim, drop
// empties/overlong/duplicates, cap counts. Model quirks are cleaned, not fatal.
func parseTerms(text string) (Terms, error) {
	jsonStr, err := extractJSONObject(text)
	if err != nil {
		return Terms{}, err
	}
	var raw Terms
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return Terms{}, fmt.Errorf("terms json: %w", err)
	}
	return Terms{
		Entities: sanitizeTerms(raw.Entities, maxEntities),
		Topics:   sanitizeTerms(raw.Topics, maxTopics),
	}, nil
}

func sanitizeTerms(in []string, max int) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, max)
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || len([]rune(s)) > maxTermRunes || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) == max {
			break
		}
	}
	return out
}

// termRows flattens extraction output into store rows; a term tagged as both
// entity and topic keeps the entity kind.
func termRows(ts Terms) []terms.Term {
	seen := make(map[string]bool, len(ts.Entities)+len(ts.Topics))
	out := make([]terms.Term, 0, len(ts.Entities)+len(ts.Topics))
	for _, e := range ts.Entities {
		if !seen[e] {
			seen[e] = true
			out = append(out, terms.Term{Term: e, Kind: "entity"})
		}
	}
	for _, t := range ts.Topics {
		if !seen[t] {
			seen[t] = true
			out = append(out, terms.Term{Term: t, Kind: "topic"})
		}
	}
	return out
}
```

（`extractJSONObject` 已存在于 `enrichment.go`，同包直接用。）

- [ ] **Step 4: 跑测试确认通过**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -run 'Extract|ParseTerms|TermRows'
```
Expected: PASS

- [ ] **Step 5: gofmt + commit**

```bash
cd server && gofmt -l internal/pipeline/
git add internal/pipeline/terms.go internal/pipeline/terms_test.go
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(pipeline): LLM 实体+话题词抽取（TermExtractor）"
```

---

### Task 4: Processor 挂载 + pulse/cmd 接线

**Files:**
- Modify: `server/internal/pipeline/processor.go`（struct + WithTermExtractor + processOne）
- Modify: `server/internal/pulse/pulse.go`（Deps + Run）
- Modify: `server/cmd/pulse/main.go`
- Test: `server/internal/pipeline/processor_terms_test.go`（Create）

- [ ] **Step 1: 写失败测试**

`server/internal/pipeline/processor_terms_test.go`（复用 helpers_test.go 的 testPool/testStores + fakeEnricher；DB 测试，无 test DB 自动 skip）：

```go
package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"aihot-server/internal/ingest"
	"aihot-server/internal/terms"
)

type fakeExtractor struct {
	out Terms
	err error
}

func (f fakeExtractor) Extract(ctx context.Context, title, summary string) (Terms, error) {
	return f.out, f.err
}

func seedRaw(t *testing.T, raw *ingest.RawStore, id string) {
	t.Helper()
	now := time.Now().UTC()
	body := "body"
	if _, err := raw.InsertIfNew(context.Background(), ingest.RawItem{
		ID: id, Source: "Example", SourceKind: "rss", URL: "https://example.com/" + id,
		Title: "title-" + id, RawContent: &body, PublishedAt: &now,
	}); err != nil {
		t.Fatalf("insert raw: %v", err)
	}
}

func validEnrichment() Enrichment {
	return Enrichment{TitleCN: "中文标题", SummaryCN: "摘要", Category: "ai-models", Relevance: 5, Score: 3}
}

func TestProcessorWritesTerms(t *testing.T) {
	pool := testPool(t)
	raw, its := testStores(t, pool)
	ts := terms.NewStore(pool)
	if err := ts.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("terms schema: %v", err)
	}
	seedRaw(t, raw, "x1")

	proc := NewProcessor(raw, its, fakeEnricher{out: validEnrichment()}).
		WithTermExtractor(fakeExtractor{out: Terms{Entities: []string{"OpenAI"}, Topics: []string{"开源"}}}, ts)
	res, err := proc.ProcessBatch(context.Background(), 10)
	if err != nil || res.Processed != 1 {
		t.Fatalf("batch: %+v err=%v", res, err)
	}

	cloud, err := ts.Cloud(context.Background(), nil, 10)
	if err != nil {
		t.Fatalf("cloud: %v", err)
	}
	if len(cloud) != 2 {
		t.Fatalf("terms not written: %+v", cloud)
	}
}

func TestProcessorTermsFailureIsBestEffort(t *testing.T) {
	pool := testPool(t)
	raw, its := testStores(t, pool)
	ts := terms.NewStore(pool)
	if err := ts.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("terms schema: %v", err)
	}
	seedRaw(t, raw, "x2")

	proc := NewProcessor(raw, its, fakeEnricher{out: validEnrichment()}).
		WithTermExtractor(fakeExtractor{err: errors.New("llm down")}, ts)
	res, err := proc.ProcessBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("batch err: %v", err)
	}
	// 抽取挂了，item 主流程必须照常成功
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("batch: %+v", res)
	}
	it, err := its.GetByID(context.Background(), "x2")
	if err != nil || it == nil {
		t.Fatalf("item not upserted: %v", err)
	}
}
```

注意：`seedRaw` 里 `ingest.RawItem` 的字段名以 `internal/ingest` 实际定义为准（写测试前先打开 `ingest` 包对一遍字段；InsertIfNew 的签名如不同，照 `ingest/store.go` 现有测试的造数方式改）。`items.Store` 如无 `GetByID`（在 `query.go`）就换成现有的取单条方法。

- [ ] **Step 2: 跑测试确认失败**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -run 'ProcessorWritesTerms|ProcessorTermsFailure'
```
Expected: FAIL（WithTermExtractor undefined）

- [ ] **Step 3: 改 processor.go**

struct 加两个字段（`translator Translator` 之后）：

```go
	termsX     TermExtractor // optional; nil disables term extraction
	termsSink  TermsSink     // where extracted terms go (item_terms)
```

`WithTranslator` 之后加：

```go
// TermsSink is the write surface term extraction needs (satisfied by *terms.Store).
type TermsSink interface {
	ReplaceForItem(ctx context.Context, itemID string, ts []terms.Term) error
}

// WithTermExtractor enables entity/topic extraction into the terms sink.
func (p *Processor) WithTermExtractor(x TermExtractor, sink TermsSink) *Processor {
	p.termsX = x
	p.termsSink = sink
	return p
}
```

import 加 `"aihot-server/internal/terms"`。

`processOne` 里，`p.items.Upsert` 成功之后、`MarkProcessed` 之前插入（terms 表有外键，必须等 item 落库；失败只记日志——这一点与 translate 的"upsert 前"模式不同，是有意的）：

```go
	if err := p.items.Upsert(ctx, it); err != nil {
		return err
	}
	// Extract entities/topics into item_terms, best-effort: extraction or the
	// sink failing must never fail the item (backfillterms can repair later).
	// Runs after Upsert because item_terms has an FK on items(id).
	if p.termsX != nil && p.termsSink != nil {
		summary := ""
		if it.Summary != nil {
			summary = *it.Summary
		}
		if ts, err := p.termsX.Extract(ctx, it.Title, summary); err != nil {
			fmt.Fprintf(os.Stderr, "terms %s: %v\n", r.ID, err)
		} else if rows := termRows(ts); len(rows) > 0 {
			if err := p.termsSink.ReplaceForItem(ctx, it.ID, rows); err != nil {
				fmt.Fprintf(os.Stderr, "terms store %s: %v\n", r.ID, err)
			}
		}
	}
	return p.raw.MarkProcessed(ctx, r.ID)
```

- [ ] **Step 4: 跑测试确认通过**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/
```
Expected: PASS（全包，含既有测试不回归）

- [ ] **Step 5: pulse 接线**

`server/internal/pulse/pulse.go`：Deps 加一个字段：

```go
	Terms      pipeline.TermExtractor // optional; enables entity/topic extraction
```

`Run` 里 itemsStore EnsureSchema 之后加：

```go
	termsStore := terms.NewStore(d.Pool)
	if err := termsStore.EnsureSchema(ctx); err != nil {
		return sum, fmt.Errorf("terms schema: %w", err)
	}
```

processor 构造改为：

```go
	proc := pipeline.NewProcessor(rawStore, itemsStore, pipeline.NewEnricher(d.LLM)).
		WithMediaResolver(ingest.NewOGResolver()).
		WithTranslator(d.Translator)
	if d.Terms != nil {
		proc = proc.WithTermExtractor(d.Terms, termsStore)
	}
```

import 加 `"aihot-server/internal/terms"`。

`server/cmd/pulse/main.go`：`translateClient` 之后加：

```go
	termsClient, err := llm.NewTermsClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "terms llm:", err)
		os.Exit(1)
	}
```

`pulse.Run` 调用的 Deps 加 `Terms: pipeline.NewTermExtractor(termsClient),`。

- [ ] **Step 6: 编译 + 全测 + commit**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./... && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test -p 1 ./internal/...
gofmt -l internal/ cmd/
git add internal/pipeline/ internal/pulse/ cmd/pulse/
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(pipeline): 抽取步骤挂进富化流水线（best-effort）+ pulse 接线"
```

---

### Task 5: cmd/backfillterms 存量回填工具

**Files:**
- Create: `server/cmd/backfillterms/main.go`

- [ ] **Step 1: 实现**（一次性 cmd，照 backfilltranslate 的先例不写单测，核心逻辑已在 Task 1/3 覆盖）

```go
// backfillterms is a one-off: it extracts entities/topics into item_terms for
// items that don't have any yet, using the low-tier terms model. Idempotent,
// safe to re-run; only touches item_terms.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... [AIHOT_TERMS_MODEL=...] ./backfillterms [-limit N]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/internal/db"
	"aihot-server/internal/llm"
	"aihot-server/internal/pipeline"
	"aihot-server/internal/terms"
)

func main() {
	limit := flag.Int("limit", 2000, "max items to extract terms for")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	client, err := llm.NewTermsClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm:", err)
		os.Exit(1)
	}
	x := pipeline.NewTermExtractor(client)

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()
	store := terms.NewStore(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}

	rows, err := pool.Query(ctx, `
		SELECT i.id, i.title, COALESCE(i.summary, '')
		FROM items i
		WHERE NOT EXISTS (SELECT 1 FROM item_terms t WHERE t.item_id = i.id)
		ORDER BY COALESCE(i.published_at,'epoch'::timestamptz) DESC
		LIMIT $1`, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	type target struct{ id, title, summary string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.title, &t.summary); err != nil {
			rows.Close()
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		targets = append(targets, t)
	}
	rows.Close()

	var done, empty, failed int
	for i, t := range targets {
		ts, err := x.Extract(ctx, t.title, t.summary)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "extract %s: %v\n", t.id, err)
			continue
		}
		rws := pipeline.TermRowsForBackfill(ts)
		if len(rws) == 0 {
			empty++
			continue
		}
		if err := store.ReplaceForItem(ctx, t.id, rws); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "store %s: %v\n", t.id, err)
			continue
		}
		done++
		if (i+1)%25 == 0 {
			fmt.Printf("progress: %d/%d done=%d empty=%d failed=%d\n", i+1, len(targets), done, empty, failed)
		}
	}
	fmt.Printf("backfillterms: targets=%d done=%d empty=%d failed=%d\n", len(targets), done, empty, failed)
}
```

`termRows` 是 pipeline 私有函数，cmd 用不了 → 在 `server/internal/pipeline/terms.go` 末尾加一个导出包装：

```go
// TermRowsForBackfill exposes the Terms→rows flattening for the backfill cmd.
func TermRowsForBackfill(ts Terms) []terms.Term { return termRows(ts) }
```

- [ ] **Step 2: 编译验证 + commit**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./cmd/backfillterms/ && gofmt -l cmd/backfillterms/ internal/pipeline/
git add cmd/backfillterms/ internal/pipeline/terms.go
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(cmd): backfillterms 存量词抽取回填"
```

---

### Task 6: publicapi graph handlers

**Files:**
- Create: `server/internal/publicapi/graph_handlers.go`
- Test: `server/internal/publicapi/graph_handlers_test.go`

- [ ] **Step 1: 写失败测试**

`server/internal/publicapi/graph_handlers_test.go`：

```go
package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aihot-server/internal/terms"
)

type fakeGraphStore struct {
	cloud     []terms.CloudTerm
	neighbors []terms.Neighbor
	items     []terms.TermItem
	kind      string
	count     int
	err       error

	gotSince *time.Time // captured from the last call
	gotTerm  string
}

func (f *fakeGraphStore) Cloud(ctx context.Context, since *time.Time, limit int) ([]terms.CloudTerm, error) {
	f.gotSince = since
	return f.cloud, f.err
}
func (f *fakeGraphStore) Neighbors(ctx context.Context, term string, since *time.Time, limit int) ([]terms.Neighbor, error) {
	f.gotTerm, f.gotSince = term, since
	return f.neighbors, f.err
}
func (f *fakeGraphStore) ItemsForTerm(ctx context.Context, term string, since *time.Time, limit int) ([]terms.TermItem, error) {
	return f.items, f.err
}
func (f *fakeGraphStore) TermInfo(ctx context.Context, term string, since *time.Time) (string, int, error) {
	return f.kind, f.count, f.err
}

var fixedNow = func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) }

func TestGraphCloudShapeAndWindow(t *testing.T) {
	f := &fakeGraphStore{cloud: []terms.CloudTerm{{Term: "OpenAI", Kind: "entity", Count: 42}}}
	h := NewGraphCloudHandler(f, fixedNow)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/cloud", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var env struct {
		Terms []map[string]any `json:"terms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Terms) != 1 || env.Terms[0]["term"] != "OpenAI" || env.Terms[0]["kind"] != "entity" || env.Terms[0]["count"].(float64) != 42 {
		t.Fatalf("terms: %+v", env.Terms)
	}
	// 默认窗口 = 7d
	want := fixedNow().Add(-7 * 24 * time.Hour)
	if f.gotSince == nil || !f.gotSince.Equal(want) {
		t.Fatalf("default since: %v", f.gotSince)
	}
}

func TestGraphCloudWindowParams(t *testing.T) {
	cases := []struct {
		q      string
		isNil  bool
		offset time.Duration
	}{
		{"window=30d", false, 30 * 24 * time.Hour},
		{"window=all", true, 0},
		{"window=bogus", false, 7 * 24 * time.Hour}, // 非法值宽容回默认
	}
	for _, c := range cases {
		f := &fakeGraphStore{}
		h := NewGraphCloudHandler(f, fixedNow)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/cloud?"+c.q, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s code: %d", c.q, rr.Code)
		}
		if c.isNil {
			if f.gotSince != nil {
				t.Fatalf("%s: since should be nil, got %v", c.q, f.gotSince)
			}
		} else if f.gotSince == nil || !f.gotSince.Equal(fixedNow().Add(-c.offset)) {
			t.Fatalf("%s: since %v", c.q, f.gotSince)
		}
	}
}

func TestGraphCloudEmptyIsEmptyArray(t *testing.T) {
	h := NewGraphCloudHandler(&fakeGraphStore{}, fixedNow)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/cloud", nil))
	if rr.Body.String() != `{"terms":[]}` {
		t.Fatalf("body: %s", rr.Body.String())
	}
}

func TestGraphTermShapeAndEscaping(t *testing.T) {
	pub := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	f := &fakeGraphStore{
		kind: "entity", count: 42,
		neighbors: []terms.Neighbor{{Term: "推理模型", Kind: "topic", Weight: 18}},
		items:     []terms.TermItem{{ID: "i1", Title: "标题", URL: "https://x/i1", Permalink: "/items/i1", Source: "S", PublishedAt: &pub, Selected: true}},
	}
	h := NewGraphTermHandler(f, fixedNow)
	rr := httptest.NewRecorder()
	// URL 编码的中文词必须解回来
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/term/%E6%8E%A8%E7%90%86?window=30d", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	if f.gotTerm != "推理" {
		t.Fatalf("term: %q", f.gotTerm)
	}
	var env struct {
		Term      string           `json:"term"`
		Kind      string           `json:"kind"`
		Count     int              `json:"count"`
		Neighbors []map[string]any `json:"neighbors"`
		Items     []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Term != "推理" || env.Kind != "entity" || env.Count != 42 {
		t.Fatalf("head: %+v", env)
	}
	if len(env.Neighbors) != 1 || env.Neighbors[0]["weight"].(float64) != 18 {
		t.Fatalf("neighbors: %+v", env.Neighbors)
	}
	if len(env.Items) != 1 || env.Items[0]["permalink"] != "/items/i1" {
		t.Fatalf("items: %+v", env.Items)
	}
}

func TestGraphTermMissingIs404(t *testing.T) {
	h := NewGraphTermHandler(&fakeGraphStore{}, fixedNow)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/term/", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("code: %d", rr.Code)
	}
}

func TestGraphTermUnknownIs200Empty(t *testing.T) {
	h := NewGraphTermHandler(&fakeGraphStore{}, fixedNow)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/graph/term/nobody", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var env struct {
		Count     int   `json:"count"`
		Neighbors []any `json:"neighbors"`
		Items     []any `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 0 || len(env.Neighbors) != 0 || len(env.Items) != 0 {
		t.Fatalf("env: %+v", env)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/publicapi/ -run Graph
```
Expected: FAIL（NewGraphCloudHandler undefined）

- [ ] **Step 3: 实现**

`server/internal/publicapi/graph_handlers.go`：

```go
package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aihot-server/internal/terms"
)

// GraphStore is the read surface the graph handlers need (satisfied by *terms.Store).
type GraphStore interface {
	Cloud(ctx context.Context, since *time.Time, limit int) ([]terms.CloudTerm, error)
	Neighbors(ctx context.Context, term string, since *time.Time, limit int) ([]terms.Neighbor, error)
	ItemsForTerm(ctx context.Context, term string, since *time.Time, limit int) ([]terms.TermItem, error)
	TermInfo(ctx context.Context, term string, since *time.Time) (kind string, count int, err error)
}

const (
	cloudLimit     = 80
	neighborLimit  = 20
	termItemsLimit = 20
)

// parseWindow maps ?window= to a since-cutoff: 7d (default, also the fallback
// for garbage) / 30d / all→nil. Lenient on purpose — public API style.
func parseWindow(v string, now time.Time) *time.Time {
	switch v {
	case "all":
		return nil
	case "30d":
		t := now.Add(-30 * 24 * time.Hour)
		return &t
	default: // "", "7d", anything else
		t := now.Add(-7 * 24 * time.Hour)
		return &t
	}
}

type cloudTermJSON struct {
	Term  string `json:"term"`
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type cloudResponse struct {
	Terms []cloudTermJSON `json:"terms"`
}

// NewGraphCloudHandler serves GET /api/public/graph/cloud?window=7d|30d|all.
func NewGraphCloudHandler(s GraphStore, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		since := parseWindow(r.URL.Query().Get("window"), now())
		rows, err := s.Cloud(r.Context(), since, cloudLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		env := cloudResponse{Terms: make([]cloudTermJSON, 0, len(rows))}
		for _, c := range rows {
			env.Terms = append(env.Terms, cloudTermJSON{Term: c.Term, Kind: c.Kind, Count: c.Count})
		}
		writeJSON(w, env)
	})
}

type neighborJSON struct {
	Term   string `json:"term"`
	Kind   string `json:"kind"`
	Weight int    `json:"weight"`
}

type termResponse struct {
	Term      string         `json:"term"`
	Kind      string         `json:"kind"`
	Count     int            `json:"count"`
	Neighbors []neighborJSON `json:"neighbors"`
	Items     []PublicItem   `json:"items"`
}

// NewGraphTermHandler serves GET /api/public/graph/term/{term}?window=….
// Unknown terms answer 200 with empty arrays (easier for the frontend); only a
// missing path segment is a 404.
func NewGraphTermHandler(s GraphStore, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.URL.Path, "/api/public/graph/term/")
		term, err := url.PathUnescape(raw)
		if err != nil || term == "" {
			writeError(w, http.StatusNotFound, "term not found")
			return
		}
		since := parseWindow(r.URL.Query().Get("window"), now())

		kind, count, err := s.TermInfo(r.Context(), term, since)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		ns, err := s.Neighbors(r.Context(), term, since, neighborLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		its, err := s.ItemsForTerm(r.Context(), term, since, termItemsLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		env := termResponse{Term: term, Kind: kind, Count: count,
			Neighbors: make([]neighborJSON, 0, len(ns)), Items: make([]PublicItem, 0, len(its))}
		for _, n := range ns {
			env.Neighbors = append(env.Neighbors, neighborJSON{Term: n.Term, Kind: n.Kind, Weight: n.Weight})
		}
		for _, it := range its {
			env.Items = append(env.Items, PublicItem{
				ID: it.ID, Title: it.Title, TitleEN: it.TitleEN, URL: it.URL,
				Permalink: it.Permalink, Source: it.Source, PublishedAt: it.PublishedAt,
				Summary: it.Summary, ImageURL: it.ImageURL, Category: it.Category,
				Score: it.Score, Selected: it.Selected,
			})
		}
		writeJSON(w, env)
	})
}

// writeJSON marshals and writes a 200 JSON body (500 on marshal failure).
func writeJSON(w http.ResponseWriter, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encoding error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, s-maxage=300, stale-while-revalidate=300")
	_, _ = w.Write(body)
}
```

（`writeError` 已存在于 publicapi 包。若包内已有等价的 writeJSON 助手，用现成的，别重复定义。）

- [ ] **Step 4: 跑测试确认通过**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/publicapi/
```
Expected: PASS

- [ ] **Step 5: gofmt + commit**

```bash
cd server && gofmt -l internal/publicapi/
git add internal/publicapi/
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(api): /api/public/graph/cloud + /graph/term/{term}"
```

---

### Task 7: 路由注册

**Files:**
- Modify: `server/common/server/httpserv/httpserv.go:127`（hot-topics 注册之后）

- [ ] **Step 1: 注册路由**

`hotStore` 段之后加：

```go
	// 图谱路由：同一个 pool；EnsureSchema 同样尽力而为（item_terms 依赖 items 表，
	// 生产库由 pulse 保证 items 已建）。
	termsStore := terms.NewStore(pool)
	if pool != nil {
		if err := termsStore.EnsureSchema(context.Background()); err != nil {
			fmt.Printf("[aihot] terms EnsureSchema failed: %v\n", err)
		}
	}
	svr.AddHTTPHandle("/api/public/graph/cloud", publicapi.NewGraphCloudHandler(termsStore, time.Now))
	svr.AddHTTPHandle("/api/public/graph/term/", publicapi.NewGraphTermHandler(termsStore, time.Now))
```

import 加 `"aihot-server/internal/terms"`。

- [ ] **Step 2: 编译 + 全测 + commit**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./... && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test
gofmt -l common/
git add common/server/httpserv/httpserv.go
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(server): 注册图谱两个公共路由"
```

---

### Task 8: 前端依赖 + api/graph.ts

**Files:**
- Modify: `web/package.json`（npm install）
- Create: `web/src/api/graph.ts`
- Test: `web/src/api/graph.test.ts`

- [ ] **Step 1: 装依赖**

```bash
cd web && npm install d3-cloud d3-force && npm install -D @types/d3-cloud @types/d3-force
```
Expected: package.json dependencies 出现 d3-cloud@^1.2.9、d3-force@^3.0.0

- [ ] **Step 2: 写失败测试**

`web/src/api/graph.test.ts`：

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fetchCloud, fetchTerm } from './graph'

describe('graph api', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('fetchCloud hits the cloud endpoint with the window', async () => {
    const mock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ terms: [] }) })
    globalThis.fetch = mock as unknown as typeof fetch
    await fetchCloud('30d')
    expect(mock).toHaveBeenCalledWith('/api/public/graph/cloud?window=30d')
  })

  it('fetchTerm URL-encodes the term', async () => {
    const mock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ term: '推理', kind: 'topic', count: 1, neighbors: [], items: [] }) })
    globalThis.fetch = mock as unknown as typeof fetch
    await fetchTerm('推理模型', '7d')
    expect(mock).toHaveBeenCalledWith(`/api/public/graph/term/${encodeURIComponent('推理模型')}?window=7d`)
  })

  it('throws on non-ok responses', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    await expect(fetchCloud('7d')).rejects.toThrow('500')
    await expect(fetchTerm('x', '7d')).rejects.toThrow('500')
  })
})
```

- [ ] **Step 3: 跑测试确认失败**

```bash
cd web && npx vitest run src/api/graph.test.ts
```
Expected: FAIL（模块不存在）

- [ ] **Step 4: 实现**

`web/src/api/graph.ts`：

```ts
import type { PublicItem } from './items'

export type GraphWindow = '7d' | '30d' | 'all'

export interface CloudTerm {
  term: string
  kind: 'entity' | 'topic'
  count: number
}

export interface CloudResponse {
  terms: CloudTerm[]
}

export interface GraphNeighbor {
  term: string
  kind: 'entity' | 'topic'
  weight: number
}

export interface TermResponse {
  term: string
  kind: string
  count: number
  neighbors: GraphNeighbor[]
  items: PublicItem[]
}

export async function fetchCloud(window: GraphWindow): Promise<CloudResponse> {
  const res = await fetch(`/api/public/graph/cloud?window=${window}`)
  if (!res.ok) throw new Error(`graph cloud fetch failed: ${res.status}`)
  return (await res.json()) as CloudResponse
}

export async function fetchTerm(term: string, window: GraphWindow): Promise<TermResponse> {
  const res = await fetch(`/api/public/graph/term/${encodeURIComponent(term)}?window=${window}`)
  if (!res.ok) throw new Error(`graph term fetch failed: ${res.status}`)
  return (await res.json()) as TermResponse
}
```

- [ ] **Step 5: 跑测试确认通过 + commit**

```bash
cd web && npx vitest run src/api/graph.test.ts
git add web/package.json web/package-lock.json web/src/api/graph.ts web/src/api/graph.test.ts
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(web): graph API 封装 + d3-cloud/d3-force 依赖"
```

---

### Task 9: 布局模块（cloudLayout / graphLayout）

**Files:**
- Create: `web/src/cloudLayout.ts`（d3-cloud 封装——jsdom 无 canvas 不可测，保持薄）
- Create: `web/src/graphLayout.ts`
- Test: `web/src/graphLayout.test.ts`

- [ ] **Step 1: 写 graphLayout 失败测试**

`web/src/graphLayout.test.ts`：

```ts
import { describe, it, expect } from 'vitest'
import { layoutGraph } from './graphLayout'

describe('layoutGraph', () => {
  it('positions every node with finite in-bounds coordinates', () => {
    const nodes = [
      { id: 'focus', r: 26 },
      { id: 'a', r: 14 },
      { id: 'b', r: 10 },
    ]
    const edges = [
      { source: 'focus', target: 'a', width: 3 },
      { source: 'focus', target: 'b', width: 1 },
    ]
    const { nodes: placed, edges: lines } = layoutGraph(nodes, edges, 400, 300)
    expect(placed).toHaveLength(3)
    for (const n of placed) {
      expect(Number.isFinite(n.x)).toBe(true)
      expect(Number.isFinite(n.y)).toBe(true)
      expect(n.x).toBeGreaterThanOrEqual(n.r)
      expect(n.x).toBeLessThanOrEqual(400 - n.r)
      expect(n.y).toBeGreaterThanOrEqual(n.r)
      expect(n.y).toBeLessThanOrEqual(300 - n.r)
    }
    expect(lines).toHaveLength(2)
    for (const l of lines) {
      expect(Number.isFinite(l.x1)).toBe(true)
      expect(Number.isFinite(l.y2)).toBe(true)
    }
  })

  it('handles a single node without edges', () => {
    const { nodes: placed, edges: lines } = layoutGraph([{ id: 'only', r: 20 }], [], 200, 200)
    expect(placed).toHaveLength(1)
    expect(lines).toHaveLength(0)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd web && npx vitest run src/graphLayout.test.ts
```
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现两个布局模块**

`web/src/graphLayout.ts`：

```ts
import {
  forceCenter,
  forceCollide,
  forceLink,
  forceManyBody,
  forceSimulation,
  type SimulationLinkDatum,
  type SimulationNodeDatum,
} from 'd3-force'

export interface GraphNode {
  id: string
  r: number
}

export interface GraphEdge {
  source: string
  target: string
  width: number
}

export interface PlacedNode extends GraphNode {
  x: number
  y: number
}

export interface PlacedEdge {
  x1: number
  y1: number
  x2: number
  y2: number
  width: number
}

type SimNode = GraphNode & SimulationNodeDatum

// layoutGraph runs a force simulation synchronously (fixed ticks, no timers) —
// deterministic enough for a static render and jsdom-safe (pure math, no DOM).
export function layoutGraph(
  nodes: GraphNode[],
  edges: GraphEdge[],
  width: number,
  height: number,
): { nodes: PlacedNode[]; edges: PlacedEdge[] } {
  const ns: SimNode[] = nodes.map((n) => ({ ...n }))
  const ls: (SimulationLinkDatum<SimNode> & { width: number })[] = edges.map((e) => ({ ...e }))

  const sim = forceSimulation(ns)
    .force('link', forceLink(ls).id((d: SimNode) => d.id).distance(90))
    .force('charge', forceManyBody().strength(-160))
    .force('center', forceCenter(width / 2, height / 2))
    .force('collide', forceCollide<SimNode>().radius((d) => d.r + 8))
    .stop()
  for (let i = 0; i < 200; i++) sim.tick()

  const clampX = (n: SimNode) => Math.min(Math.max(n.x ?? 0, n.r), width - n.r)
  const clampY = (n: SimNode) => Math.min(Math.max(n.y ?? 0, n.r), height - n.r)
  const placed = ns.map((n) => ({ id: n.id, r: n.r, x: clampX(n), y: clampY(n) }))
  const byID = new Map(placed.map((n) => [n.id, n]))

  const lines: PlacedEdge[] = ls.map((l) => {
    // forceLink resolves source/target strings into node objects during tick
    const s = byID.get(typeof l.source === 'object' ? (l.source as SimNode).id : String(l.source))
    const t = byID.get(typeof l.target === 'object' ? (l.target as SimNode).id : String(l.target))
    return { x1: s?.x ?? 0, y1: s?.y ?? 0, x2: t?.x ?? 0, y2: t?.y ?? 0, width: l.width }
  })
  return { nodes: placed, edges: lines }
}
```

`web/src/cloudLayout.ts`：

```ts
import cloud from 'd3-cloud'

export interface CloudWord {
  text: string
  size: number
  kind: string
}

export interface PlacedWord extends CloudWord {
  x: number
  y: number
}

// layoutCloud runs the d3-cloud (Wordle) packing. Needs canvas for text
// measurement, so component tests mock this module. Words that don't fit the
// box are silently dropped by d3-cloud — acceptable for the tail of the top-80.
export function layoutCloud(words: CloudWord[], width: number, height: number): Promise<PlacedWord[]> {
  return new Promise((resolve) => {
    cloud()
      .size([width, height])
      .words(words.map((w) => ({ ...w })))
      .padding(3)
      .rotate(0)
      .font('system-ui')
      .fontSize((d) => d.size ?? 12)
      .on('end', (out) => resolve(out as unknown as PlacedWord[]))
      .start()
  })
}
```

（`@types/d3-cloud` 的 `Word` 类型字段全是可选的，`as unknown as PlacedWord[]` 是这里唯一妥协；若 oxlint 报风格问题按提示调。）

- [ ] **Step 4: 跑测试确认通过 + commit**

```bash
cd web && npx vitest run src/graphLayout.test.ts && npx tsc -b
git add web/src/cloudLayout.ts web/src/graphLayout.ts web/src/graphLayout.test.ts
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(web): 词云/力导向布局模块"
```

---

### Task 10: TermPanel 组件（子图 + 相关资讯）

**Files:**
- Create: `web/src/components/TermPanel.tsx`
- Test: `web/src/components/TermPanel.test.tsx`

- [ ] **Step 1: 写失败测试**

`web/src/components/TermPanel.test.tsx`：

```tsx
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { TermPanel } from './TermPanel'

const termPayload = {
  term: 'OpenAI', kind: 'entity', count: 42,
  neighbors: [
    { term: '推理模型', kind: 'topic', weight: 18 },
    { term: '开源', kind: 'topic', weight: 5 },
  ],
  items: [
    { id: 'i1', title: '大新闻', url: 'https://x/i1', permalink: '/items/i1', source: 'OpenAI Blog', selected: true, publishedAt: '2026-07-10T08:00:00Z' },
  ],
}

describe('TermPanel', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders subgraph neighbors and related items', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => termPayload }) as unknown as typeof fetch
    render(<TermPanel term="OpenAI" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
    expect(globalThis.fetch).toHaveBeenCalledWith(`/api/public/graph/term/${encodeURIComponent('OpenAI')}?window=7d`)
    expect(screen.getByRole('link', { name: '大新闻' })).toHaveAttribute('href', '/items/i1')
    expect(screen.getByText(/42 条相关/)).toBeInTheDocument()
  })

  it('clicking a neighbor switches focus via onSelect', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => termPayload }) as unknown as typeof fetch
    const onSelect = vi.fn()
    render(<TermPanel term="OpenAI" window="7d" onSelect={onSelect} />)
    await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
    fireEvent.click(screen.getByText('推理模型'))
    expect(onSelect).toHaveBeenCalledWith('推理模型')
  })

  it('shows the empty state when the term has no data in the window', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ term: 'x', kind: '', count: 0, neighbors: [], items: [] }) }) as unknown as typeof fetch
    render(<TermPanel term="x" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText(/该时间段暂无数据/)).toBeInTheDocument())
  })

  it('shows the error state when the fetch fails', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    render(<TermPanel term="x" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText(/加载失败/)).toBeInTheDocument())
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd web && npx vitest run src/components/TermPanel.test.tsx
```
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现**

`web/src/components/TermPanel.tsx`：

```tsx
import { useEffect, useState } from 'react'
import { fetchTerm, type GraphWindow, type TermResponse } from '../api/graph'
import { layoutGraph, type GraphEdge, type GraphNode } from '../graphLayout'
import { formatDate } from '../format'

const W = 420
const H = 320
const FOCUS_R = 26

// TermPanel is the drill-down under the cloud: a static force-directed
// co-occurrence subgraph (click a neighbor to refocus) + the term's newest items.
export function TermPanel({
  term, window: win, onSelect,
}: {
  term: string
  window: GraphWindow
  onSelect: (term: string) => void
}) {
  const [data, setData] = useState<TermResponse | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setData(null)
    setFailed(false)
    fetchTerm(term, win)
      .then(setData)
      .catch(() => setFailed(true))
  }, [term, win])

  if (failed) return <div className="gv-panel gv-status">加载失败，稍后再试</div>
  if (!data) return <div className="gv-panel gv-status">加载中…</div>
  if (data.count === 0) return <div className="gv-panel gv-status">该时间段暂无数据</div>

  const maxW = Math.max(...data.neighbors.map((n) => n.weight), 1)
  const nodes: GraphNode[] = [
    { id: data.term, r: FOCUS_R },
    ...data.neighbors.map((n) => ({ id: n.term, r: 10 + 12 * (n.weight / maxW) })),
  ]
  const edges: GraphEdge[] = data.neighbors.map((n) => ({
    source: data.term, target: n.term, width: 1 + 3 * (n.weight / maxW),
  }))
  const g = layoutGraph(nodes, edges, W, H)
  const kindOf = new Map(data.neighbors.map((n) => [n.term, n.kind]))

  return (
    <section className="gv-panel">
      <div className="gv-subgraph">
        <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${data.term} 关联图`}>
          {g.edges.map((e, i) => (
            <line key={i} x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2} className="gv-edge" strokeWidth={e.width} />
          ))}
          {g.nodes.map((n) => {
            const isFocus = n.id === data.term
            const kind = isFocus ? data.kind : kindOf.get(n.id)
            return (
              <g
                key={n.id}
                className={`gv-node${isFocus ? ' focus' : ''} ${kind === 'entity' ? 'entity' : 'topic'}`}
                onClick={() => { if (!isFocus) onSelect(n.id) }}
              >
                <circle cx={n.x} cy={n.y} r={n.r} />
                <text x={n.x} y={n.y + n.r + 12} textAnchor="middle">{n.id}</text>
              </g>
            )
          })}
        </svg>
      </div>
      <div className="gv-items">
        <h3>「{data.term}」 · {data.count} 条相关</h3>
        {data.items.map((it) => (
          <div key={it.id} className="gv-item-row">
            <a href={it.permalink}>{it.title}</a>
            <span className="gv-item-meta">
              {it.source}
              {it.publishedAt ? ` · ${formatDate(it.publishedAt)}` : ''}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}
```

注意：`formatDate` 以 `web/src/format.ts` 里实际导出的日期格式化函数为准（打开看一眼签名；若名字不同就用现有的那个，不要新写）。

- [ ] **Step 4: 跑测试确认通过 + commit**

```bash
cd web && npx vitest run src/components/TermPanel.test.tsx
git add web/src/components/TermPanel.tsx web/src/components/TermPanel.test.tsx
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(web): TermPanel 共现子图 + 相关资讯面板"
```

---

### Task 11: GraphView 组件（词云 + 窗口切换）

**Files:**
- Create: `web/src/components/GraphView.tsx`
- Test: `web/src/components/GraphView.test.tsx`

- [ ] **Step 1: 写失败测试**

`web/src/components/GraphView.test.tsx`（mock `cloudLayout` 模块——jsdom 无 canvas）：

```tsx
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { CloudWord } from '../cloudLayout'
import { GraphView } from './GraphView'

vi.mock('../cloudLayout', () => ({
  layoutCloud: async (words: CloudWord[]) =>
    words.map((w, i) => ({ ...w, x: i * 60 - 100, y: 0 })),
}))

const cloudPayload = {
  terms: [
    { term: 'OpenAI', kind: 'entity', count: 42 },
    { term: '开源', kind: 'topic', count: 7 },
  ],
}
const termPayload = {
  term: 'OpenAI', kind: 'entity', count: 42,
  neighbors: [{ term: '推理模型', kind: 'topic', weight: 3 }],
  items: [{ id: 'i1', title: '大新闻', url: 'https://x/i1', permalink: '/items/i1', source: 'S', selected: true }],
}

function mockFetch() {
  return vi.fn().mockImplementation((url: string) => {
    if (String(url).includes('/graph/term/')) {
      return Promise.resolve({ ok: true, json: async () => termPayload })
    }
    return Promise.resolve({ ok: true, json: async () => cloudPayload })
  }) as unknown as typeof fetch
}

describe('GraphView', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders the cloud words after loading', async () => {
    globalThis.fetch = mockFetch()
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText('OpenAI')).toBeInTheDocument())
    expect(screen.getByText('开源')).toBeInTheDocument()
    expect(globalThis.fetch).toHaveBeenCalledWith('/api/public/graph/cloud?window=7d')
  })

  it('clicking a word opens the term panel', async () => {
    globalThis.fetch = mockFetch()
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText('OpenAI')).toBeInTheDocument())
    fireEvent.click(screen.getByText('OpenAI'))
    await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
    expect(globalThis.fetch).toHaveBeenCalledWith(`/api/public/graph/term/${encodeURIComponent('OpenAI')}?window=7d`)
  })

  it('switching the window refetches the cloud', async () => {
    globalThis.fetch = mockFetch()
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText('OpenAI')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '30 天' }))
    await waitFor(() =>
      expect(globalThis.fetch).toHaveBeenCalledWith('/api/public/graph/cloud?window=30d'))
  })

  it('shows the empty state for an empty window', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ terms: [] }) }) as unknown as typeof fetch
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText(/该时间段暂无数据/)).toBeInTheDocument())
  })

  it('shows the error state when the cloud fetch fails', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText(/加载失败/)).toBeInTheDocument())
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd web && npx vitest run src/components/GraphView.test.tsx
```
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现**

`web/src/components/GraphView.tsx`：

```tsx
import { useEffect, useState } from 'react'
import { fetchCloud, type CloudTerm, type GraphWindow } from '../api/graph'
import { layoutCloud, type PlacedWord } from '../cloudLayout'
import { TermPanel } from './TermPanel'

const CLOUD_W = 900
const CLOUD_H = 420
const MIN_FONT = 14
const MAX_FONT = 48

const WINDOWS: { key: GraphWindow; label: string }[] = [
  { key: '7d', label: '7 天' },
  { key: '30d', label: '30 天' },
  { key: 'all', label: '全部' },
]

// fontSize maps count→px on a sqrt scale so the head doesn't drown the tail.
function fontSize(count: number, maxCount: number): number {
  return MIN_FONT + (MAX_FONT - MIN_FONT) * Math.sqrt(count / maxCount)
}

// GraphView is the 「图谱」 main view: word cloud entry (size = frequency in
// the window), click a word to drill into its co-occurrence panel.
export function GraphView() {
  const [win, setWin] = useState<GraphWindow>('7d')
  const [words, setWords] = useState<PlacedWord[] | null>(null)
  const [empty, setEmpty] = useState(false)
  const [failed, setFailed] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)

  useEffect(() => {
    let stale = false
    setWords(null)
    setEmpty(false)
    setFailed(false)
    fetchCloud(win)
      .then(async (res) => {
        if (stale) return
        if (res.terms.length === 0) {
          setEmpty(true)
          return
        }
        const maxCount = res.terms[0].count
        const placed = await layoutCloud(
          res.terms.map((t: CloudTerm) => ({ text: t.term, size: fontSize(t.count, maxCount), kind: t.kind })),
          CLOUD_W, CLOUD_H,
        )
        if (!stale) setWords(placed)
      })
      .catch(() => { if (!stale) setFailed(true) })
    return () => { stale = true }
  }, [win])

  return (
    <div className="graph-page">
      <header className="page-head">
        <h1>图谱</h1>
        <p className="page-sub">词云看热度，点一个词看它跟谁连着</p>
      </header>

      <div className="gv-controls" role="group" aria-label="时间范围">
        {WINDOWS.map((w) => (
          <button
            key={w.key}
            className={`gv-win-btn${win === w.key ? ' active' : ''}`}
            aria-pressed={win === w.key}
            onClick={() => setWin(w.key)}
          >
            {w.label}
          </button>
        ))}
      </div>

      {failed && <div className="gv-status">加载失败，稍后再试</div>}
      {empty && <div className="gv-status">该时间段暂无数据</div>}
      {!failed && !empty && !words && <div className="gv-status">加载中…</div>}

      {words && (
        <svg
          className="gv-cloud"
          viewBox={`${-CLOUD_W / 2} ${-CLOUD_H / 2} ${CLOUD_W} ${CLOUD_H}`}
          role="img"
          aria-label="关键词云图"
        >
          {words.map((w) => (
            <text
              key={w.text}
              x={w.x}
              y={w.y}
              fontSize={w.size}
              textAnchor="middle"
              className={`gv-word ${w.kind === 'entity' ? 'entity' : 'topic'}${selected === w.text ? ' selected' : ''}`}
              onClick={() => setSelected(w.text)}
            >
              {w.text}
            </text>
          ))}
        </svg>
      )}

      {selected && <TermPanel term={selected} window={win} onSelect={setSelected} />}
    </div>
  )
}
```

（d3-cloud 的坐标系原点在画布中心，viewBox 用负偏移对齐。）

- [ ] **Step 4: 跑测试确认通过 + commit**

```bash
cd web && npx vitest run src/components/GraphView.test.tsx
git add web/src/components/GraphView.tsx web/src/components/GraphView.test.tsx
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(web): GraphView 词云主视图"
```

---

### Task 12: 接线（Sidebar / App / CSS）

**Files:**
- Modify: `web/src/components/Sidebar.tsx:3-9`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/index.css`（追加）

- [ ] **Step 1: Sidebar 加视图**

```ts
export type View = 'selected' | 'all' | 'daily' | 'graph'

const NAV: { view: View; label: string; icon: string }[] = [
  { view: 'selected', label: '精选', icon: '✦' },
  { view: 'all', label: '全部 AI 动态', icon: '≣' },
  { view: 'daily', label: 'AI 日报', icon: '▤' },
  { view: 'graph', label: '图谱', icon: '❖' },
]
```

- [ ] **Step 2: App 加分支**

`web/src/App.tsx`：HEADERS 的类型收窄改成排除两个自带头部的视图，渲染加 graph 分支：

```tsx
import { GraphView } from './components/GraphView'

const HEADERS: Record<Exclude<View, 'daily' | 'graph'>, { title: string; sub: string }> = {
  selected: { title: '精选', sub: 'AI 自动挑选的高价值内容' },
  all: { title: '全部 AI 动态', sub: 'AI 相关资讯全量信息流' },
}
```

```tsx
        {view === 'daily' ? (
          <DailyView />
        ) : view === 'graph' ? (
          <GraphView />
        ) : (
          <>
            <header className="page-head">
              <h1>{HEADERS[view].title}</h1>
              <p className="page-sub">{HEADERS[view].sub}</p>
            </header>
            <Feed mode={view} />
          </>
        )}
```

- [ ] **Step 3: App.test 补 graph**

`web/src/App.test.tsx`：文件顶部（import 之后）加 cloudLayout mock；`mockAll` 里在 hot-topics 分支之前加 graph 分支；文件尾加一个切换测试：

```tsx
vi.mock('./cloudLayout', () => ({
  layoutCloud: async (words: { text: string; size: number; kind: string }[]) =>
    words.map((w, i) => ({ ...w, x: i * 60 - 100, y: 0 })),
}))
```

```tsx
    if (u.includes('/api/public/graph/cloud')) {
      return Promise.resolve({ ok: true, json: async () => ({ terms: [{ term: '词云词', kind: 'topic', count: 3 }] }) })
    }
```

```tsx
it('switches to the graph view', async () => {
  mockAll()
  render(<App />)
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())
  fireEvent.click(screen.getByRole('button', { name: '图谱' }))
  await waitFor(() => expect(screen.getByText('词云词')).toBeInTheDocument())
  expect(screen.queryByText('标题A')).toBeNull()
})
```

- [ ] **Step 4: CSS**

`web/src/index.css` 末尾追加（600px 断点段放在文件已有的 `@media (max-width: 600px)` 里，如无独立段就新开一个）：

```css
/* ---- 图谱视图 ---- */
.gv-controls { display: flex; gap: 8px; margin: 4px 0 16px; }
.gv-win-btn {
  padding: 4px 14px; border: 1px solid var(--border); border-radius: 999px;
  background: var(--chip); color: var(--text); font-size: 13px; cursor: pointer;
}
.gv-win-btn.active { background: var(--accent-soft); border-color: var(--accent); color: var(--accent); }

.gv-cloud { width: 100%; height: auto; display: block; }
.gv-word { cursor: pointer; font-family: var(--sans); font-weight: 600; }
.gv-word.entity { fill: var(--accent); }
.gv-word.topic { fill: var(--gold); }
.gv-word:hover, .gv-word.selected { opacity: .7; text-decoration: underline; }

.gv-status { color: var(--muted); padding: 32px 0; text-align: center; }

.gv-panel {
  display: grid; grid-template-columns: 1fr 1fr; gap: 20px;
  border-top: 1px solid var(--border); margin-top: 20px; padding-top: 20px;
}
.gv-subgraph svg { width: 100%; height: auto; }
.gv-edge { stroke: var(--rail); }
.gv-node { cursor: pointer; }
.gv-node.focus { cursor: default; }
.gv-node.entity circle { fill: var(--accent); }
.gv-node.topic circle { fill: var(--gold); }
.gv-node.focus circle { stroke: var(--text); stroke-width: 2; }
.gv-node text { fill: var(--text); font-size: 12px; }

.gv-items h3 { font-size: 15px; margin: 0 0 10px; }
.gv-item-row { padding: 7px 0; border-bottom: 1px solid var(--border); display: flex; flex-direction: column; gap: 2px; }
.gv-item-row a { color: var(--text); text-decoration: none; font-size: 14px; }
.gv-item-row a:hover { color: var(--accent); }
.gv-item-meta { color: var(--muted); font-size: 12px; }

@media (max-width: 600px) {
  .gv-panel { grid-template-columns: 1fr; }
}
```

- [ ] **Step 5: 全量前端验证 + commit**

```bash
cd web && npx vitest run && npm run build && npm run lint
git add web/src/
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(web): 侧栏图谱入口 + App 接线 + 样式"
```
Expected: vitest 全绿、tsc/vite build 通过、oxlint 干净

---

### Task 13: 全量验证

- [ ] **Step 1: 后端全测**

```bash
cd server && gofmt -l . | grep -v '^common/' ; GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test
```
Expected: gofmt 无新增脏文件；make test 全绿（DB 测试有 AIHOT_TEST_DATABASE_URL 时必须真跑一次）

- [ ] **Step 2: 前端全测 + 构建**

```bash
cd web && npx vitest run && npm run build
```
Expected: PASS

- [ ] **Step 3:（可选，本地有 DB 时）端到端冒烟**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go run main.go   # 起服务后另开窗口
curl -s 'http://localhost:8991/api/public/graph/cloud?window=all' | head -c 300
curl -s 'http://localhost:8991/api/public/graph/term/OpenAI?window=all' | head -c 300
```
Expected: 两个端点返回 JSON（本地库没跑过抽取时 terms 为空数组，也算通）

- [ ] **Step 4: 收尾 commit（若有散落改动）**

```bash
git status --short   # 应该干净；有遗漏则补交
```

---

### Task 14: 部署 Melos + 存量回填（操作性任务，逐条执行并验证）

前置：Task 1-13 全部完成且 main 上测试全绿。连接一律 `ssh -o ClearAllForwardings=yes melos`。

- [ ] **Step 1: 交叉编译**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/aihot-build/server . \
  && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/aihot-build/pulse ./cmd/pulse \
  && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/aihot-build/backfillterms ./cmd/backfillterms
```

- [ ] **Step 2: 原子替换二进制 + 重启**

```bash
scp -o ClearAllForwardings=yes /tmp/aihot-build/server melos:/root/aihot/bin/server.new
scp -o ClearAllForwardings=yes /tmp/aihot-build/pulse melos:/root/aihot/bin/pulse.new
scp -o ClearAllForwardings=yes /tmp/aihot-build/backfillterms melos:/root/aihot/bin/backfillterms.new
ssh -o ClearAllForwardings=yes melos 'chmod +x /root/aihot/bin/*.new && mv -f /root/aihot/bin/server.new /root/aihot/bin/server && mv -f /root/aihot/bin/pulse.new /root/aihot/bin/pulse && mv -f /root/aihot/bin/backfillterms.new /root/aihot/bin/backfillterms && supervisorctl restart aihot-server'
```

- [ ] **Step 3: 发前端**

```bash
cd web && npm run build && rsync -e 'ssh -o ClearAllForwardings=yes' -av --delete dist/ melos:/var/www/aihot/
```

- [ ] **Step 4: 冒烟**

```bash
curl -s 'http://10.190.12.242:8899/api/public/graph/cloud?window=all' | head -c 200
```
Expected: `{"terms":[]}`（还没回填，空数组正常；500 则查 supervisor 日志）

- [ ] **Step 5: nohup 回填（环境变量照 aihot-cron.sh 的 export 方式，key 绝不落屏）**

```bash
ssh -o ClearAllForwardings=yes melos 'set -a; source /root/wechat-push/.env; set +a; export AIHOT_LLM_API_KEY="$LLM_API_KEY" AIHOT_DATABASE_URL=postgres://aihot:aihot@localhost:5432/aihot; nohup /root/aihot/bin/backfillterms -limit 2000 > /root/aihot/backfillterms.log 2>&1 & echo started'
```

- [ ] **Step 6: poll 进度（每隔几分钟看一次，别前台 sleep 等）**

```bash
ssh -o ClearAllForwardings=yes melos 'tail -5 /root/aihot/backfillterms.log'
```
Expected: 最终一行 `backfillterms: targets=~977 done=… empty=… failed=…`，failed 占比 <5%

- [ ] **Step 7: 线上验证**

```bash
curl -s 'http://10.190.12.242:8899/api/public/graph/cloud?window=all' | head -c 400
```
Expected: terms 非空、count 排序合理。然后浏览器打开 http://10.190.12.242:8899/ → 图谱视图，人工点两个词看子图和资讯列表。

- [ ] **Step 8: 观察下一次 pulse（:30 每半小时）**

```bash
ssh -o ClearAllForwardings=yes melos 'grep -i "terms" /root/aihot/logs/* 2>/dev/null | tail; psql postgres://aihot:aihot@localhost:5432/aihot -c "SELECT count(*) FROM item_terms"'
```
Expected: 新 item 进来后 item_terms 行数随之增长；有 `terms <id>: ...` 报错行属 best-effort 正常，大面积报错才需要查。

---

## Self-Review 结论（已跑）

- **Spec 覆盖**：数据层(T1)、抽取(T3/T4)、回填(T5/T14)、API(T6/T7)、词云(T9/T11)、子图+资讯(T10)、入口+窗口切换(T11/T12)、移动端(T12 CSS)、空态/错误(T6/T10/T11)、测试(各任务+T13)、发布(T14)——全覆盖。
- **外部事实已验证**：d3-cloud@1.2.9 / @types/d3-cloud@1.2.9 / d3-force@3.0.0 / @types/d3-force@3.0.10 在 npm 可拉；`TRUNCATE items` 因新外键需 CASCADE 已入 Task 1。
- **已知留给执行者核对的点**（都标在任务里）：`ingest.RawItem` 造数字段、`items.Store.GetByID`、`format.ts` 的日期函数名、publicapi 是否已有 writeJSON 助手。执行到该任务时先打开对应文件对一遍再写。
