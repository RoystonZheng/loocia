# Daily Report (日报主编⑦) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the daily-report feature end-to-end: a generator that reads the day's selected items, groups them into category sections, has the LLM write a lead (导语), and stores the report; the three read endpoints (`/api/public/daily`, `/api/public/daily/{date}`, `/api/public/dailies`); a `cmd/gendaily` manual trigger; and a front-end 日报 view toggled from the feed.

**Architecture:** Sections are **deterministic** (group selected items by category in a fixed order, score-desc within); the LLM writes **only the lead** — one call, and on LLM failure the report is stored with `lead: null` (contract-legal) so the daily never fails because of the model. Reports are stored in a `dailies` table with the openapi wire shape serialized as JSONB (API reads become trivial). The `items` store gains an `Until` param for window queries `[date 00:00Z, +24h)`. Zero selected items in the window → no report stored (preserves the 404 semantics). Front-end adds a state-toggled 日报 view (no router yet).

**Tech Stack:** Go (`internal/daily`, `internal/publicapi` additions, `cmd/gendaily`), PostgreSQL JSONB, the existing `internal/llm` client, React/vitest for the front-end.

---

## Environment / how to run

- Go via `server/Makefile` (`GOTOOLCHAIN=go1.25.5`); single package: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org [envs] go test ./internal/daily/ -run <name> -v`.
- Live PG via the SSH tunnel on `localhost:5432` (being re-established; verify with `nc -z localhost 5432` before DB tasks; restart with `ssh -N -L 5432:localhost:5432 melos &` if down). DSN `postgres://aihot:aihot@localhost:5432/aihot_test`.
- LLM: internal proxy via `internal/llm.NewClientFromEnv` (env `AIHOT_LLM_API_KEY`; fetch inline: `export AIHOT_LLM_API_KEY=$(ssh melos 'grep ^LLM_API_KEY /root/wechat-push/.env | cut -d= -f2-' | tr -d '"'\'' \r')`). Real-LLM tests SKIP without the key. **Never print/commit the key.**
- Front-end: `cd web && npx vitest run` / `npm run build` (hermetic).

## Contract recap (docs/references/openapi.yaml)

- `DailyReport`: `{date "YYYY-MM-DD", generatedAt, windowStart, windowEnd, lead:{title,leadParagraph}|null, sections:[{label, items:[{title,summary,sourceUrl,sourceName,permalink|null}]}], flashes:[{title,sourceName,sourceUrl,publishedAt,permalink|null}]}`.
- `GET /api/public/daily` → latest report; 404 暂无.
- `GET /api/public/daily/{date}` → `^\d{4}-\d{2}-\d{2}$` else 400; 404 该日无.
- `GET /api/public/dailies?take=` → strict int 1–180 default 30 else 400; `{count, items:[{date, generatedAt, leadTitle|null, leadParagraph|null}]}`.
- Section labels: ai-models→模型发布/更新, ai-products→产品发布/更新, industry→行业动态, paper→论文研究, tip→技巧与观点.
- Date basis: UTC 0点 (`YYYY-MM-DD（UTC 0 点为基准）`).

## File Structure

- `server/internal/items/query.go` — add `Until *time.Time` to `ListParams` (+ predicate).
- `server/internal/daily/report.go` — `Report`/`Section`/`SectionItem`/`Flash`/`Lead`/`Entry` wire models, category label/order.
- `server/internal/daily/schema.sql`, `store.go` — `dailies` table + `Store` (Upsert/GetLatest/GetByDate/ListRecent, `ErrNotFound`).
- `server/internal/daily/build.go` — `buildSections`, `buildFlashes` (deterministic), `LLM` iface, `generateLead`+`parseLead`.
- `server/internal/daily/generator.go` — `Generator.Generate(ctx, date, now)`; `ErrNoItems`.
- `server/cmd/gendaily/main.go` — manual trigger.
- `server/internal/publicapi/daily_handlers.go` — the 3 handlers + `DailyStore` iface.
- `server/common/server/httpserv/httpserv.go` — mount routes.
- `web/src/api/daily.ts`, `web/src/components/DailyView.tsx`, `web/src/App.tsx` — front-end.

---

### Task 1: items store — `Until` window bound

**Files:**
- Modify: `server/internal/items/query.go`
- Modify: `server/internal/items/query_test.go` (append test)

- [ ] **Step 1: Write the failing test** — append to `server/internal/items/query_test.go`:
```go
func TestListUntilExcludesLaterItems(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	seed(t, s,
		sampleItem("in-window", base.Add(2*time.Hour)),
		sampleItem("at-bound", base.Add(24*time.Hour)),  // == until → excluded (half-open)
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
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org AIHOT_TEST_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' go test ./internal/items/ -run TestListUntil -v` → FAIL (`unknown field Until`).

- [ ] **Step 3: Implement** — in `server/internal/items/query.go`: add `Until *time.Time` to `ListParams` (after `Since`, comment `// exclusive upper bound on published_at`); in the SQL add a predicate line after the `$3` since-line:
```
		  AND ($8::timestamptz IS NULL OR published_at < $8)
```
and append `p.Until` as the 8th query argument (AFTER `limit`; placeholder order in SQL text is independent of argument position — `LIMIT $7` stays as-is, `$8` is simply the eighth arg).

- [ ] **Step 4: Run to confirm PASS** — same command → PASS. Then whole items package `-run . -count=1` → all PASS (no regression).

- [ ] **Step 5: Commit**
```bash
git add server/internal/items/
git commit -m "feat(items): Until window bound on List"
```

---

### Task 2: daily models + store

**Files:**
- Create: `server/internal/daily/report.go`
- Create: `server/internal/daily/schema.sql`
- Create: `server/internal/daily/store.go`
- Create: `server/internal/daily/helpers_test.go`
- Test: `server/internal/daily/store_test.go`

- [ ] **Step 1: Write models + schema + store**

`server/internal/daily/report.go`:
```go
package daily

import "time"

// Wire models mirror docs/references/openapi.yaml DailyReport / DailyEntries.

type SectionItem struct {
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	SourceURL  string  `json:"sourceUrl"`
	SourceName string  `json:"sourceName"`
	Permalink  *string `json:"permalink"`
}

type Section struct {
	Label string        `json:"label"`
	Items []SectionItem `json:"items"`
}

type Flash struct {
	Title       string     `json:"title"`
	SourceName  string     `json:"sourceName"`
	SourceURL   string     `json:"sourceUrl"`
	PublishedAt *time.Time `json:"publishedAt"`
	Permalink   *string    `json:"permalink"`
}

type Lead struct {
	Title         string `json:"title"`
	LeadParagraph string `json:"leadParagraph"`
}

type Report struct {
	Date        string    `json:"date"` // YYYY-MM-DD (UTC day)
	GeneratedAt time.Time `json:"generatedAt"`
	WindowStart time.Time `json:"windowStart"`
	WindowEnd   time.Time `json:"windowEnd"`
	Lead        *Lead     `json:"lead"`
	Sections    []Section `json:"sections"`
	Flashes     []Flash   `json:"flashes"`
}

// Entry is one row of the dailies archive index (openapi DailyEntries.items).
type Entry struct {
	Date          string    `json:"date"`
	GeneratedAt   time.Time `json:"generatedAt"`
	LeadTitle     *string   `json:"leadTitle"`
	LeadParagraph *string   `json:"leadParagraph"`
}

// categoryOrder is the fixed section order; categoryLabels maps slug → 中文 label
// (matches the openapi items↔daily correspondence table).
var categoryOrder = []string{"ai-models", "ai-products", "industry", "paper", "tip"}

var categoryLabels = map[string]string{
	"ai-models":   "模型发布/更新",
	"ai-products": "产品发布/更新",
	"industry":    "行业动态",
	"paper":       "论文研究",
	"tip":         "技巧与观点",
}
```

`server/internal/daily/schema.sql`:
```sql
CREATE TABLE IF NOT EXISTS dailies (
    date           TEXT PRIMARY KEY,  -- YYYY-MM-DD; ISO text orders correctly
    generated_at   TIMESTAMPTZ NOT NULL,
    window_start   TIMESTAMPTZ NOT NULL,
    window_end     TIMESTAMPTZ NOT NULL,
    lead_title     TEXT,
    lead_paragraph TEXT,
    sections       JSONB NOT NULL,
    flashes        JSONB NOT NULL
);
```

`server/internal/daily/store.go`:
```go
package daily

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// ErrNotFound is returned when no report matches.
var ErrNotFound = errors.New("daily: not found")

// Store persists daily reports.
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

// Upsert stores the report, replacing any existing report for the same date.
func (s *Store) Upsert(ctx context.Context, r Report) error {
	sections, err := json.Marshal(r.Sections)
	if err != nil {
		return fmt.Errorf("marshal sections: %w", err)
	}
	flashes, err := json.Marshal(r.Flashes)
	if err != nil {
		return fmt.Errorf("marshal flashes: %w", err)
	}
	var leadTitle, leadParagraph *string
	if r.Lead != nil {
		leadTitle, leadParagraph = &r.Lead.Title, &r.Lead.LeadParagraph
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO dailies (date, generated_at, window_start, window_end, lead_title, lead_paragraph, sections, flashes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (date) DO UPDATE SET
			generated_at=EXCLUDED.generated_at, window_start=EXCLUDED.window_start,
			window_end=EXCLUDED.window_end, lead_title=EXCLUDED.lead_title,
			lead_paragraph=EXCLUDED.lead_paragraph, sections=EXCLUDED.sections, flashes=EXCLUDED.flashes`,
		r.Date, r.GeneratedAt, r.WindowStart, r.WindowEnd, leadTitle, leadParagraph, sections, flashes)
	return err
}

const reportColumns = `date, generated_at, window_start, window_end, lead_title, lead_paragraph, sections, flashes`

func (s *Store) GetLatest(ctx context.Context) (*Report, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+reportColumns+` FROM dailies ORDER BY date DESC LIMIT 1`)
	return scanReport(row)
}

func (s *Store) GetByDate(ctx context.Context, date string) (*Report, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+reportColumns+` FROM dailies WHERE date=$1`, date)
	return scanReport(row)
}

func scanReport(row pgx.Row) (*Report, error) {
	var r Report
	var leadTitle, leadParagraph *string
	var sections, flashes []byte
	err := row.Scan(&r.Date, &r.GeneratedAt, &r.WindowStart, &r.WindowEnd, &leadTitle, &leadParagraph, &sections, &flashes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if leadTitle != nil && leadParagraph != nil {
		r.Lead = &Lead{Title: *leadTitle, LeadParagraph: *leadParagraph}
	}
	if err := json.Unmarshal(sections, &r.Sections); err != nil {
		return nil, fmt.Errorf("unmarshal sections: %w", err)
	}
	if err := json.Unmarshal(flashes, &r.Flashes); err != nil {
		return nil, fmt.Errorf("unmarshal flashes: %w", err)
	}
	return &r, nil
}

// ListRecent returns up to take newest-first archive entries.
func (s *Store) ListRecent(ctx context.Context, take int) ([]Entry, error) {
	if take <= 0 {
		take = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT date, generated_at, lead_title, lead_paragraph
		FROM dailies ORDER BY date DESC LIMIT $1`, take)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Date, &e.GeneratedAt, &e.LeadTitle, &e.LeadParagraph); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 2: Write the tests**

`server/internal/daily/helpers_test.go`:
```go
package daily

import (
	"context"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
)

func newTestStore(t *testing.T) *Store {
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
	s := NewStore(pool)
	if err := s.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE dailies"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func sampleReport(date string) Report {
	gen := time.Date(2026, 5, 8, 1, 0, 0, 0, time.UTC)
	ws := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	perma := "/items/x1"
	pub := ws.Add(2 * time.Hour)
	return Report{
		Date: date, GeneratedAt: gen, WindowStart: ws, WindowEnd: ws.Add(24 * time.Hour),
		Lead: &Lead{Title: "今日导语", LeadParagraph: "这是导语段落。"},
		Sections: []Section{{
			Label: "模型发布/更新",
			Items: []SectionItem{{Title: "条目一", Summary: "摘要一", SourceURL: "https://x/1", SourceName: "Src", Permalink: &perma}},
		}},
		Flashes: []Flash{{Title: "快讯一", SourceName: "Src", SourceURL: "https://x/1", PublishedAt: &pub, Permalink: &perma}},
	}
}
```

`server/internal/daily/store_test.go`:
```go
package daily

import (
	"context"
	"errors"
	"testing"
)

func TestUpsertThenGetRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	in := sampleReport("2026-05-07")
	if err := s.Upsert(ctx, in); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := s.GetByDate(ctx, "2026-05-07")
	if err != nil {
		t.Fatalf("GetByDate: %v", err)
	}
	if got.Lead == nil || got.Lead.Title != "今日导语" {
		t.Fatalf("lead: %+v", got.Lead)
	}
	if len(got.Sections) != 1 || got.Sections[0].Label != "模型发布/更新" || len(got.Sections[0].Items) != 1 {
		t.Fatalf("sections: %+v", got.Sections)
	}
	if got.Sections[0].Items[0].Permalink == nil || *got.Sections[0].Items[0].Permalink != "/items/x1" {
		t.Fatalf("permalink: %+v", got.Sections[0].Items[0])
	}
	if len(got.Flashes) != 1 || got.Flashes[0].PublishedAt == nil {
		t.Fatalf("flashes: %+v", got.Flashes)
	}
	if !got.WindowStart.Equal(in.WindowStart) || !got.WindowEnd.Equal(in.WindowEnd) {
		t.Fatalf("window: %v %v", got.WindowStart, got.WindowEnd)
	}
}

func TestUpsertReplacesSameDate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Upsert(ctx, sampleReport("2026-05-07")); err != nil {
		t.Fatal(err)
	}
	updated := sampleReport("2026-05-07")
	updated.Lead = nil // regeneration without lead
	updated.Sections = nil
	if err := s.Upsert(ctx, updated); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	got, err := s.GetByDate(ctx, "2026-05-07")
	if err != nil {
		t.Fatal(err)
	}
	if got.Lead != nil {
		t.Fatalf("lead should be nil after replace: %+v", got.Lead)
	}
	if len(got.Sections) != 0 {
		t.Fatalf("sections should be empty after replace: %+v", got.Sections)
	}
}

func TestGetLatestAndListRecent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, d := range []string{"2026-05-05", "2026-05-07", "2026-05-06"} {
		if err := s.Upsert(ctx, sampleReport(d)); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := s.GetLatest(ctx)
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if latest.Date != "2026-05-07" {
		t.Fatalf("latest: %s", latest.Date)
	}
	entries, err := s.ListRecent(ctx, 2)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(entries) != 2 || entries[0].Date != "2026-05-07" || entries[1].Date != "2026-05-06" {
		t.Fatalf("entries: %+v", entries)
	}
	if entries[0].LeadTitle == nil || *entries[0].LeadTitle != "今日导语" {
		t.Fatalf("entry leadTitle: %+v", entries[0])
	}
}

func TestNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetLatest(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetLatest empty: %v", err)
	}
	if _, err := s.GetByDate(context.Background(), "2026-01-01"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByDate missing: %v", err)
	}
}
```

- [ ] **Step 3: Run** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org AIHOT_TEST_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' go test ./internal/daily/ -v` → PASS (4). Without DSN → SKIP.

- [ ] **Step 4: Build + commit**
```bash
cd server && make build
git add server/internal/daily/
git commit -m "feat(daily): report models + dailies store"
```

---

### Task 3: builder — sections, flashes, LLM lead

**Files:**
- Create: `server/internal/daily/build.go`
- Test: `server/internal/daily/build_test.go`

- [ ] **Step 1: Write the failing test**

`server/internal/daily/build_test.go`:
```go
package daily

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aihot-server/internal/items"
)

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

func mkSelItem(id, cat string, score int, pub time.Time) items.Item {
	summary := "摘要-" + id
	c := cat
	sc := score
	return items.Item{
		ID: id, Title: "标题-" + id, URL: "https://x/" + id, Permalink: "/items/" + id,
		Source: "Src", PublishedAt: &pub, Summary: &summary, Category: &c, Score: &sc,
		Selected: true, Present: true,
	}
}

func TestBuildSectionsGroupsAndOrders(t *testing.T) {
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)
	its := []items.Item{
		mkSelItem("p1", "paper", 70, base),
		mkSelItem("m1", "ai-models", 80, base),
		mkSelItem("m2", "ai-models", 95, base.Add(time.Hour)),
	}
	secs := buildSections(its)
	// Fixed order: ai-models before paper; empty categories skipped.
	if len(secs) != 2 || secs[0].Label != "模型发布/更新" || secs[1].Label != "论文研究" {
		t.Fatalf("section order: %+v", secs)
	}
	// Within section: score desc → m2 (95) before m1 (80).
	if secs[0].Items[0].Title != "标题-m2" || secs[0].Items[1].Title != "标题-m1" {
		t.Fatalf("in-section order: %+v", secs[0].Items)
	}
	// Field mapping.
	it0 := secs[0].Items[0]
	if it0.Summary != "摘要-m2" || it0.SourceURL != "https://x/m2" || it0.SourceName != "Src" {
		t.Fatalf("mapping: %+v", it0)
	}
	if it0.Permalink == nil || *it0.Permalink != "/items/m2" {
		t.Fatalf("permalink: %+v", it0)
	}
}

func TestBuildSectionsSkipsUncategorized(t *testing.T) {
	base := time.Date(2026, 5, 7, 8, 0, 0, 0, time.UTC)
	noCat := mkSelItem("x", "ai-models", 50, base)
	noCat.Category = nil
	secs := buildSections([]items.Item{noCat})
	if len(secs) != 0 {
		t.Fatalf("uncategorized should be skipped: %+v", secs)
	}
}

func TestBuildFlashesTakesMostRecent(t *testing.T) {
	base := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	var its []items.Item
	for i := 0; i < 7; i++ {
		its = append(its, mkSelItem(string(rune('a'+i)), "industry", 60, base.Add(time.Duration(i)*time.Hour)))
	}
	fl := buildFlashes(its, 5)
	if len(fl) != 5 {
		t.Fatalf("want 5 flashes, got %d", len(fl))
	}
	// Most recent first: g (6h) then f, e, d, c.
	if fl[0].Title != "标题-g" || fl[4].Title != "标题-c" {
		t.Fatalf("flash order: %+v", fl)
	}
}

type fakeLLM struct {
	reply string
	err   error
}

func (f fakeLLM) Complete(ctx context.Context, system, user string) (string, error) {
	return f.reply, f.err
}

func TestGenerateLeadParsesJSON(t *testing.T) {
	f := fakeLLM{reply: `{"title":"AI 圈今日看点","lead_paragraph":"今天模型层面动作频繁。"}`}
	lead, err := generateLead(context.Background(), f, []Section{{Label: "模型发布/更新", Items: []SectionItem{{Title: "t", Summary: "s"}}}})
	if err != nil {
		t.Fatalf("generateLead: %v", err)
	}
	if lead.Title != "AI 圈今日看点" || !strings.Contains(lead.LeadParagraph, "模型") {
		t.Fatalf("lead: %+v", lead)
	}
}

func TestGenerateLeadErrors(t *testing.T) {
	if _, err := generateLead(context.Background(), fakeLLM{err: errors.New("boom")}, nil); err == nil {
		t.Fatal("LLM error should propagate")
	}
	if _, err := generateLead(context.Background(), fakeLLM{reply: "not json"}, nil); err == nil {
		t.Fatal("garbage reply should error")
	}
	if _, err := generateLead(context.Background(), fakeLLM{reply: `{"title":"","lead_paragraph":"p"}`}, nil); err == nil {
		t.Fatal("empty title should error")
	}
}
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/daily/ -run 'TestBuild|TestGenerateLead' -v` → FAIL (`undefined: buildSections`).

- [ ] **Step 3: Implement**

`server/internal/daily/build.go`:
```go
package daily

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

// buildSections groups items by category in the fixed order, score-desc
// (nil score last), publishedAt-desc tiebreak. Uncategorized items are skipped.
// Empty categories produce no section.
func buildSections(its []items.Item) []Section {
	byCat := map[string][]items.Item{}
	for _, it := range its {
		if it.Category == nil {
			continue
		}
		byCat[*it.Category] = append(byCat[*it.Category], it)
	}
	var out []Section
	for _, cat := range categoryOrder {
		group := byCat[cat]
		if len(group) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			si, sj := -1, -1
			if group[i].Score != nil {
				si = *group[i].Score
			}
			if group[j].Score != nil {
				sj = *group[j].Score
			}
			if si != sj {
				return si > sj
			}
			return group[i].SortKey().After(group[j].SortKey())
		})
		sec := Section{Label: categoryLabels[cat]}
		for _, it := range group {
			sec.Items = append(sec.Items, toSectionItem(it))
		}
		out = append(out, sec)
	}
	return out
}

func toSectionItem(it items.Item) SectionItem {
	summary := ""
	if it.Summary != nil {
		summary = *it.Summary
	}
	perma := it.Permalink
	return SectionItem{
		Title:      it.Title,
		Summary:    summary,
		SourceURL:  it.URL,
		SourceName: it.Source,
		Permalink:  &perma,
	}
}

// buildFlashes returns the n most recent items (publishedAt desc) as 快讯.
func buildFlashes(its []items.Item, n int) []Flash {
	sorted := make([]items.Item, len(its))
	copy(sorted, its)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SortKey().After(sorted[j].SortKey())
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	var out []Flash
	for _, it := range sorted {
		perma := it.Permalink
		out = append(out, Flash{
			Title:       it.Title,
			SourceName:  it.Source,
			SourceURL:   it.URL,
			PublishedAt: it.PublishedAt,
			Permalink:   &perma,
		})
	}
	return out
}

const leadSystemPrompt = `你是 AI 资讯日报主编。给定今天日报的分组条目清单，写一个导语。输出一个 JSON 对象（只输出 JSON），字段：
- "title": 简短有信息量的日报标题（中文）
- "lead_paragraph": 一段 2-4 句的中文导语，概括今天最值得关注的动向`

// generateLead asks the LLM for the day's lead over the built sections.
func generateLead(ctx context.Context, l LLM, secs []Section) (*Lead, error) {
	var b strings.Builder
	for _, s := range secs {
		b.WriteString("## " + s.Label + "\n")
		for _, it := range s.Items {
			b.WriteString("- " + it.Title)
			if it.Summary != "" {
				b.WriteString("：" + it.Summary)
			}
			b.WriteString("\n")
		}
	}
	out, err := l.Complete(ctx, leadSystemPrompt, b.String())
	if err != nil {
		return nil, err
	}
	return parseLead(out)
}

type leadPayload struct {
	Title         string `json:"title"`
	LeadParagraph string `json:"lead_paragraph"`
}

// parseLead extracts and validates the lead JSON (lenient about code fences).
func parseLead(text string) (*Lead, error) {
	s := strings.TrimSpace(text)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object in lead output")
	}
	var p leadPayload
	if err := json.Unmarshal([]byte(s[start:end+1]), &p); err != nil {
		return nil, fmt.Errorf("lead json: %w", err)
	}
	if p.Title == "" {
		return nil, fmt.Errorf("empty lead title")
	}
	return &Lead{Title: p.Title, LeadParagraph: p.LeadParagraph}, nil
}
```

- [ ] **Step 4: Run to confirm PASS** — same command → PASS (all six).

- [ ] **Step 5: Commit**
```bash
git add server/internal/daily/build.go server/internal/daily/build_test.go
git commit -m "feat(daily): section/flash builders + LLM lead"
```

---

### Task 4: Generator + cmd/gendaily

**Files:**
- Create: `server/internal/daily/generator.go`
- Test: `server/internal/daily/generator_test.go`
- Create: `server/cmd/gendaily/main.go`

- [ ] **Step 1: Write the failing test**

`server/internal/daily/generator_test.go`:
```go
package daily

import (
	"context"
	"errors"
	"testing"
	"time"

	"aihot-server/internal/items"
)

// seedItems inserts selected items into the live items table for the window.
func seedItems(t *testing.T, itemsStore *items.Store, its ...items.Item) {
	t.Helper()
	for _, it := range its {
		if err := itemsStore.Upsert(context.Background(), it); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func newLiveStores(t *testing.T) (*Store, *items.Store) {
	t.Helper()
	s := newTestStore(t) // daily store, schema + truncate (skips w/o DSN)
	itemsStore := items.New(s.pool)
	if err := itemsStore.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("items EnsureSchema: %v", err)
	}
	if _, err := s.pool.Exec(context.Background(), "TRUNCATE items"); err != nil {
		t.Fatalf("truncate items: %v", err)
	}
	return s, itemsStore
}

func TestGenerateBuildsAndStoresReport(t *testing.T) {
	s, itemsStore := newLiveStores(t)
	ctx := context.Background()
	day := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)

	seedItems(t, itemsStore,
		mkSelItem("in1", "ai-models", 90, day.Add(2*time.Hour)),
		mkSelItem("in2", "industry", 75, day.Add(20*time.Hour)),
		mkSelItem("out-after", "ai-models", 99, day.Add(25*time.Hour)), // outside window
	)
	// Unselected item in window must be excluded.
	unsel := mkSelItem("unsel", "ai-models", 88, day.Add(3*time.Hour))
	unsel.Selected = false
	seedItems(t, itemsStore, unsel)

	g := NewGenerator(itemsStore, s, fakeLLM{reply: `{"title":"今日AI","lead_paragraph":"导语。"}`})
	now := day.Add(24*time.Hour + time.Hour)
	rep, err := g.Generate(ctx, "2026-05-07", now)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if rep.Date != "2026-05-07" || !rep.WindowStart.Equal(day) || !rep.WindowEnd.Equal(day.Add(24*time.Hour)) {
		t.Fatalf("window: %+v", rep)
	}
	if rep.Lead == nil || rep.Lead.Title != "今日AI" {
		t.Fatalf("lead: %+v", rep.Lead)
	}
	// Only the 2 in-window selected items; out-after and unsel excluded.
	total := 0
	for _, sec := range rep.Sections {
		total += len(sec.Items)
	}
	if total != 2 || len(rep.Sections) != 2 {
		t.Fatalf("sections: %+v", rep.Sections)
	}
	if len(rep.Flashes) != 2 {
		t.Fatalf("flashes: %+v", rep.Flashes)
	}

	// Stored and readable.
	got, err := s.GetByDate(ctx, "2026-05-07")
	if err != nil {
		t.Fatalf("GetByDate after generate: %v", err)
	}
	if got.Lead == nil || got.Lead.Title != "今日AI" {
		t.Fatalf("stored lead: %+v", got.Lead)
	}
}

func TestGenerateLLMFailureYieldsNilLead(t *testing.T) {
	s, itemsStore := newLiveStores(t)
	day := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	seedItems(t, itemsStore, mkSelItem("in1", "ai-models", 90, day.Add(2*time.Hour)))

	g := NewGenerator(itemsStore, s, fakeLLM{err: errors.New("llm down")})
	rep, err := g.Generate(context.Background(), "2026-05-07", day.Add(25*time.Hour))
	if err != nil {
		t.Fatalf("Generate should not fail on LLM error: %v", err)
	}
	if rep.Lead != nil {
		t.Fatalf("lead should be nil on LLM failure: %+v", rep.Lead)
	}
	if len(rep.Sections) != 1 {
		t.Fatalf("sections should still build: %+v", rep.Sections)
	}
}

func TestGenerateNoItemsReturnsErrNoItems(t *testing.T) {
	s, itemsStore := newLiveStores(t)
	g := NewGenerator(itemsStore, s, fakeLLM{reply: `{}`})
	_, err := g.Generate(context.Background(), "2026-05-07", time.Date(2026, 5, 8, 1, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrNoItems) {
		t.Fatalf("want ErrNoItems, got %v", err)
	}
	// Nothing stored → GetByDate 404s.
	if _, err := s.GetByDate(context.Background(), "2026-05-07"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no report should be stored: %v", err)
	}
}

func TestGenerateRejectsBadDate(t *testing.T) {
	s, itemsStore := newLiveStores(t)
	g := NewGenerator(itemsStore, s, fakeLLM{})
	if _, err := g.Generate(context.Background(), "07/05/2026", time.Now()); err == nil {
		t.Fatal("bad date should error")
	}
}
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org AIHOT_TEST_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' go test ./internal/daily/ -run TestGenerate -v` → FAIL (`undefined: NewGenerator` / `ErrNoItems`; the TestGenerateLead* from Task 3 still pass).

- [ ] **Step 3: Implement**

`server/internal/daily/generator.go`:
```go
package daily

import (
	"context"
	"errors"
	"fmt"
	"time"

	"aihot-server/internal/items"
)

// ErrNoItems means the window had no selected items; no report is stored.
var ErrNoItems = errors.New("daily: no selected items in window")

const (
	maxWindowItems = 200
	flashCount     = 5
)

// Generator builds and stores one day's report.
type Generator struct {
	items *items.Store
	store *Store
	llm   LLM
}

func NewGenerator(itemsStore *items.Store, store *Store, l LLM) *Generator {
	return &Generator{items: itemsStore, store: store, llm: l}
}

// Generate builds the report for the UTC day `date` (YYYY-MM-DD), stores it,
// and returns it. The LLM writes only the lead; on LLM failure the report is
// stored with a nil lead. Zero selected items → ErrNoItems, nothing stored.
func (g *Generator) Generate(ctx context.Context, date string, now time.Time) (*Report, error) {
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("bad date %q: %w", date, err)
	}
	ws := day.UTC()
	we := ws.Add(24 * time.Hour)

	tru := true
	its, err := g.items.List(ctx, items.ListParams{
		Selected: &tru, Since: &ws, Until: &we, Limit: maxWindowItems,
	})
	if err != nil {
		return nil, fmt.Errorf("list window items: %w", err)
	}
	if len(its) == 0 {
		return nil, ErrNoItems
	}

	secs := buildSections(its)
	rep := Report{
		Date:        date,
		GeneratedAt: now.UTC(),
		WindowStart: ws,
		WindowEnd:   we,
		Sections:    secs,
		Flashes:     buildFlashes(its, flashCount),
	}
	// Lead is best-effort: a model failure must not sink the daily.
	if lead, err := generateLead(ctx, g.llm, secs); err == nil {
		rep.Lead = lead
	}

	if err := g.store.Upsert(ctx, rep); err != nil {
		return nil, fmt.Errorf("store report: %w", err)
	}
	return &rep, nil
}
```

`server/cmd/gendaily/main.go`:
```go
// gendaily generates (or regenerates) the daily report for one UTC day.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... \
//	  go run ./cmd/gendaily [-date 2026-05-07]
//
// Without AIHOT_LLM_API_KEY the report is generated with a nil lead.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"aihot-server/internal/daily"
	"aihot-server/internal/db"
	"aihot-server/internal/items"
	"aihot-server/internal/llm"
)

// noLLM makes gendaily usable without a key: lead generation just fails soft.
type noLLM struct{}

func (noLLM) Complete(ctx context.Context, system, user string) (string, error) {
	return "", fmt.Errorf("no AIHOT_LLM_API_KEY: lead skipped")
}

func main() {
	date := flag.String("date", time.Now().UTC().Format("2006-01-02"), "UTC day YYYY-MM-DD")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := daily.NewStore(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}

	var model daily.LLM = noLLM{}
	if c, err := llm.NewClientFromEnv(); err == nil {
		model = c
	}

	g := daily.NewGenerator(items.New(pool), store, model)
	rep, err := g.Generate(ctx, *date, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate:", err)
		os.Exit(1)
	}
	leadTitle := "(无导语)"
	if rep.Lead != nil {
		leadTitle = rep.Lead.Title
	}
	total := 0
	for _, s := range rep.Sections {
		total += len(s.Items)
	}
	fmt.Printf("daily %s: %d sections, %d items, %d flashes, lead=%q\n",
		rep.Date, len(rep.Sections), total, len(rep.Flashes), leadTitle)
}
```

- [ ] **Step 4: Run to confirm PASS** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org AIHOT_TEST_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' go test ./internal/daily/ -v` → all PASS. `make build` → green (also compiles cmd/gendaily).

- [ ] **Step 5: Optional gated real-LLM smoke** — with the key exported (see Environment) and the DB up:
`cd server && AIHOT_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' AIHOT_LLM_API_KEY=$AIHOT_LLM_API_KEY GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go run ./cmd/gendaily -date $(date -u +%F)` → prints a summary line with a real lead title (needs selected items in today's window — the smoke rows seeded in P1.5 qualify if generated today). If no items: `generate: daily: no selected items in window` is the correct behavior, note it.

- [ ] **Step 6: Commit**
```bash
git add server/internal/daily/generator.go server/internal/daily/generator_test.go server/cmd/
git commit -m "feat(daily): generator + gendaily command"
```

---

### Task 5: API endpoints + mount

**Files:**
- Create: `server/internal/publicapi/daily_handlers.go`
- Test: `server/internal/publicapi/daily_handlers_test.go`
- Modify: `server/common/server/httpserv/httpserv.go`

- [ ] **Step 1: Write the failing test**

`server/internal/publicapi/daily_handlers_test.go`:
```go
package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aihot-server/internal/daily"
)

type fakeDailyStore struct {
	latest  *daily.Report
	byDate  map[string]*daily.Report
	entries []daily.Entry
}

func (f *fakeDailyStore) GetLatest(ctx context.Context) (*daily.Report, error) {
	if f.latest == nil {
		return nil, daily.ErrNotFound
	}
	return f.latest, nil
}
func (f *fakeDailyStore) GetByDate(ctx context.Context, date string) (*daily.Report, error) {
	if r, ok := f.byDate[date]; ok {
		return r, nil
	}
	return nil, daily.ErrNotFound
}
func (f *fakeDailyStore) ListRecent(ctx context.Context, take int) ([]daily.Entry, error) {
	if take < len(f.entries) {
		return f.entries[:take], nil
	}
	return f.entries, nil
}

func mkReport(date string) *daily.Report {
	return &daily.Report{
		Date: date, GeneratedAt: time.Date(2026, 5, 8, 1, 0, 0, 0, time.UTC),
		WindowStart: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
		Lead:        &daily.Lead{Title: "导语标题", LeadParagraph: "导语段落"},
		Sections:    []daily.Section{{Label: "模型发布/更新", Items: []daily.SectionItem{{Title: "t1", Summary: "s1", SourceURL: "https://x/1", SourceName: "Src"}}}},
		Flashes:     []daily.Flash{},
	}
}

func TestLatestDaily200And404(t *testing.T) {
	f := &fakeDailyStore{latest: mkReport("2026-05-07")}
	h := NewLatestDailyHandler(f)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/daily", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var rep daily.Report
	if err := json.Unmarshal(rr.Body.Bytes(), &rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rep.Date != "2026-05-07" || rep.Lead == nil || rep.Lead.Title != "导语标题" {
		t.Fatalf("report: %+v", rep)
	}

	empty := NewLatestDailyHandler(&fakeDailyStore{})
	rr2 := httptest.NewRecorder()
	empty.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/public/daily", nil))
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr2.Code)
	}
}

func TestDailyByDateValidations(t *testing.T) {
	f := &fakeDailyStore{byDate: map[string]*daily.Report{"2026-05-07": mkReport("2026-05-07")}}
	h := NewDailyByDateHandler(f)

	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, httptest.NewRequest(http.MethodGet, "/api/public/daily/2026-05-07", nil))
	if ok.Code != http.StatusOK {
		t.Fatalf("valid date: %d", ok.Code)
	}

	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/public/daily/07-05-2026", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad format want 400, got %d", bad.Code)
	}

	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/public/daily/2026-01-01", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing want 404, got %d", missing.Code)
	}
}

func TestDailiesIndexAndTakeValidation(t *testing.T) {
	lt := "L1"
	f := &fakeDailyStore{entries: []daily.Entry{
		{Date: "2026-05-07", GeneratedAt: time.Now().UTC(), LeadTitle: &lt},
		{Date: "2026-05-06", GeneratedAt: time.Now().UTC()},
	}}
	h := NewDailiesHandler(f)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/dailies", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var env struct {
		Count int           `json:"count"`
		Items []daily.Entry `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 2 || len(env.Items) != 2 || env.Items[0].Date != "2026-05-07" {
		t.Fatalf("envelope: %+v", env)
	}

	one := httptest.NewRecorder()
	h.ServeHTTP(one, httptest.NewRequest(http.MethodGet, "/api/public/dailies?take=1", nil))
	var env1 struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(one.Body.Bytes(), &env1)
	if env1.Count != 1 {
		t.Fatalf("take=1: %+v", env1)
	}

	for _, bad := range []string{"0", "181", "1.5", "abc"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/dailies?take="+bad, nil))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("take=%s want 400, got %d", bad, rr.Code)
		}
	}
}
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/publicapi/ -run 'TestLatestDaily|TestDailyByDate|TestDailiesIndex' -v` → FAIL (`undefined: NewLatestDailyHandler`).

- [ ] **Step 3: Implement**

`server/internal/publicapi/daily_handlers.go`:
```go
package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"aihot-server/internal/daily"
)

// DailyStore is the read surface the daily handlers need (satisfied by *daily.Store).
type DailyStore interface {
	GetLatest(ctx context.Context) (*daily.Report, error)
	GetByDate(ctx context.Context, date string) (*daily.Report, error)
	ListRecent(ctx context.Context, take int) ([]daily.Entry, error)
}

func writeDailyJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// NewLatestDailyHandler serves GET /api/public/daily.
func NewLatestDailyHandler(s DailyStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rep, err := s.GetLatest(r.Context())
		if errors.Is(err, daily.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no daily report available")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeDailyJSON(w, rep)
	})
}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// NewDailyByDateHandler serves GET /api/public/daily/{date} (mounted at the
// "/api/public/daily/" subtree; the date is the path suffix).
func NewDailyByDateHandler(s DailyStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := strings.TrimPrefix(r.URL.Path, "/api/public/daily/")
		if !datePattern.MatchString(date) {
			writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
			return
		}
		rep, err := s.GetByDate(r.Context(), date)
		if errors.Is(err, daily.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no daily report for that date")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeDailyJSON(w, rep)
	})
}

// dailiesEnvelope is the openapi DailyEntries shape.
type dailiesEnvelope struct {
	Count int           `json:"count"`
	Items []daily.Entry `json:"items"`
}

// NewDailiesHandler serves GET /api/public/dailies (take: strict int 1-180, default 30).
func NewDailiesHandler(s DailyStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		take := 30
		if t := r.URL.Query().Get("take"); t != "" {
			n, err := strconv.Atoi(t)
			if err != nil || n < 1 || n > 180 {
				writeError(w, http.StatusBadRequest, "take must be an integer 1-180")
				return
			}
			take = n
		}
		entries, err := s.ListRecent(r.Context(), take)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if entries == nil {
			entries = []daily.Entry{}
		}
		writeDailyJSON(w, dailiesEnvelope{Count: len(entries), Items: entries})
	})
}
```

- [ ] **Step 4: Run to confirm PASS** — same command → PASS (all three). Whole publicapi package `-run .` → all PASS.

- [ ] **Step 5: Mount the routes.** In `server/common/server/httpserv/httpserv.go`, next to the existing mounts, build a `daily.NewStore(pool)` (same shared pool; `EnsureSchema` best-effort like the items store) and register:
```go
dailyStore := daily.NewStore(pool)
svr.AddHTTPHandle("/api/public/daily", publicapi.NewLatestDailyHandler(dailyStore))
svr.AddHTTPHandle("/api/public/daily/", publicapi.NewDailyByDateHandler(dailyStore))
svr.AddHTTPHandle("/api/public/dailies", publicapi.NewDailiesHandler(dailyStore))
```
READ the file first and follow the exact pattern used for the items mount (pool nil-tolerance included: with a nil pool the daily routes 500 at request time, server still boots). `http.ServeMux` gives the bare `/api/public/daily` exact-match priority over the `/api/public/daily/` subtree — both registrations coexist.

- [ ] **Step 6: Live boot smoke** — with the tunnel up:
```
cd server && AIHOT_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' make run &
# wait for Server Running! addr::8991
curl -si localhost:8991/api/public/daily | head -3          # 200 (if a report exists from Task 4's smoke) or 404 — both valid
curl -s 'localhost:8991/api/public/dailies?take=5'
curl -si localhost:8991/api/public/daily/1999-13-99 | head -1   # 400
# kill the server (port 8991)
```

- [ ] **Step 7: Commit**
```bash
git add server/internal/publicapi/daily_handlers.go server/internal/publicapi/daily_handlers_test.go server/common/server/httpserv/httpserv.go
git commit -m "feat(publicapi): daily endpoints + mount"
```

---

### Task 6: Front-end 日报 view

**Files:**
- Create: `web/src/api/daily.ts`
- Test: `web/src/api/daily.test.ts`
- Create: `web/src/components/DailyView.tsx`
- Test: `web/src/components/DailyView.test.tsx`
- Modify: `web/src/App.tsx`, `web/src/App.test.tsx`, `web/src/index.css`

- [ ] **Step 1: Client + test**

`web/src/api/daily.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest'
import { fetchLatestDaily } from './daily'

describe('fetchLatestDaily', () => {
  it('returns the report', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z',
        windowStart: '2026-05-07T00:00:00Z', windowEnd: '2026-05-08T00:00:00Z',
        lead: { title: '导语', leadParagraph: '段落' }, sections: [], flashes: [],
      }),
    }) as unknown as typeof fetch
    const rep = await fetchLatestDaily()
    expect(rep?.date).toBe('2026-05-07')
    expect(rep?.lead?.title).toBe('导语')
  })

  it('returns null on 404', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 404 }) as unknown as typeof fetch
    expect(await fetchLatestDaily()).toBeNull()
  })

  it('throws on other errors', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    await expect(fetchLatestDaily()).rejects.toThrow(/500/)
  })
})
```

`web/src/api/daily.ts`:
```ts
export interface DailySectionItem {
  title: string
  summary: string
  sourceUrl: string
  sourceName: string
  permalink?: string | null
}

export interface DailySection {
  label: string
  items: DailySectionItem[]
}

export interface DailyFlash {
  title: string
  sourceName: string
  sourceUrl: string
  publishedAt?: string | null
  permalink?: string | null
}

export interface DailyReport {
  date: string
  generatedAt: string
  windowStart: string
  windowEnd: string
  lead: { title: string; leadParagraph: string } | null
  sections: DailySection[]
  flashes: DailyFlash[]
}

// fetchLatestDaily returns the latest report, or null when none exists (404).
export async function fetchLatestDaily(): Promise<DailyReport | null> {
  const res = await fetch('/api/public/daily')
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`daily fetch failed: ${res.status}`)
  return (await res.json()) as DailyReport
}
```

Run: `cd web && npx vitest run src/api/daily.test.ts` → RED first (missing module), then GREEN after writing the client.

- [ ] **Step 2: DailyView + test**

`web/src/components/DailyView.test.tsx`:
```tsx
import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { DailyView } from './DailyView'

const report = {
  date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z',
  windowStart: '2026-05-07T00:00:00Z', windowEnd: '2026-05-08T00:00:00Z',
  lead: { title: '今日AI看点', leadParagraph: '模型层面动作频繁。' },
  sections: [{
    label: '模型发布/更新',
    items: [{ title: '条目一', summary: '摘要一', sourceUrl: 'https://x/1', sourceName: 'SrcA', permalink: '/items/1' }],
  }],
  flashes: [{ title: '快讯一', sourceName: 'SrcB', sourceUrl: 'https://x/2', publishedAt: '2026-05-07T04:00:00Z', permalink: '/items/2' }],
}

describe('DailyView', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders lead, sections, flashes', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => report }) as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText('今日AI看点')).toBeInTheDocument())
    expect(screen.getByText('模型层面动作频繁。')).toBeInTheDocument()
    expect(screen.getByText('模型发布/更新')).toBeInTheDocument()
    expect(screen.getByText('条目一')).toBeInTheDocument()
    expect(screen.getByText('摘要一')).toBeInTheDocument()
    expect(screen.getByText('快讯一')).toBeInTheDocument()
    // item links to permalink
    expect(screen.getByRole('link', { name: '条目一' })).toHaveAttribute('href', '/items/1')
    // date shown
    expect(screen.getByText(/2026-05-07/)).toBeInTheDocument()
  })

  it('shows empty state on 404', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 404 }) as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText(/暂无日报/)).toBeInTheDocument())
  })

  it('shows error state on failure', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText(/加载失败/)).toBeInTheDocument())
  })
})
```

`web/src/components/DailyView.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { fetchLatestDaily, type DailyReport } from '../api/daily'
import { formatBeijingTime } from '../format'

type State = 'loading' | 'ready' | 'empty' | 'error'

export function DailyView() {
  const [state, setState] = useState<State>('loading')
  const [report, setReport] = useState<DailyReport | null>(null)

  useEffect(() => {
    fetchLatestDaily()
      .then((rep) => {
        if (rep === null) {
          setState('empty')
        } else {
          setReport(rep)
          setState('ready')
        }
      })
      .catch(() => setState('error'))
  }, [])

  if (state === 'loading') return <p className="daily-status">加载中…</p>
  if (state === 'empty') return <p className="daily-status">暂无日报。</p>
  if (state === 'error') return <p className="daily-status feed-error">加载失败，请稍后重试。</p>

  const rep = report!
  return (
    <div className="daily">
      <div className="daily-date">{rep.date} · 生成于 {formatBeijingTime(rep.generatedAt)}</div>
      {rep.lead && (
        <section className="daily-lead">
          <h2>{rep.lead.title}</h2>
          <p>{rep.lead.leadParagraph}</p>
        </section>
      )}
      {rep.sections.map((sec) => (
        <section key={sec.label} className="daily-section">
          <h3>{sec.label}</h3>
          {sec.items.map((it, i) => (
            <div key={i} className="daily-item">
              {it.permalink ? (
                <a className="item-title" href={it.permalink}>{it.title}</a>
              ) : (
                <span className="item-title">{it.title}</span>
              )}
              {it.summary && <p className="item-summary">{it.summary}</p>}
              <span className="item-meta">{it.sourceName}</span>
            </div>
          ))}
        </section>
      ))}
      {rep.flashes.length > 0 && (
        <section className="daily-section">
          <h3>快讯</h3>
          {rep.flashes.map((f, i) => (
            <div key={i} className="daily-flash">
              {f.permalink ? <a href={f.permalink}>{f.title}</a> : <span>{f.title}</span>}
              <span className="item-meta"> {f.sourceName}{f.publishedAt ? ' · ' + formatBeijingTime(f.publishedAt) : ''}</span>
            </div>
          ))}
        </section>
      )}
    </div>
  )
}
```

Run: `cd web && npx vitest run src/components/DailyView.test.tsx` → RED first, GREEN after.

- [ ] **Step 3: App nav toggle + test.** Update `web/src/App.test.tsx` — REPLACE with:
```tsx
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'

function mockAll() {
  globalThis.fetch = vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    if (u.includes('/api/public/version')) {
      return Promise.resolve({ ok: true, json: async () => ({ apiVersion: '1.1.0', skillVersion: '0.1.0', updatedAt: '2026-07-03', changelogUrl: '/changelog', recentChanges: [] }) })
    }
    if (u.includes('/api/public/daily')) {
      return Promise.resolve({ ok: true, json: async () => ({ date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z', windowStart: '2026-05-07T00:00:00Z', windowEnd: '2026-05-08T00:00:00Z', lead: { title: '日报导语', leadParagraph: '段落' }, sections: [], flashes: [] }) })
    }
    return Promise.resolve({ ok: true, json: async () => ({ count: 1, hasNext: false, nextCursor: null, items: [{ id: 'a', title: '标题A', url: 'https://x/a', permalink: '/items/a', source: 'S', selected: true }] }) })
  }) as unknown as typeof fetch
}

it('renders the feed by default and switches to the daily view', async () => {
  mockAll()
  render(<App />)
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())

  fireEvent.click(screen.getByRole('button', { name: '日报' }))
  await waitFor(() => expect(screen.getByText('日报导语')).toBeInTheDocument())
  expect(screen.queryByText('标题A')).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: '资讯流' }))
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())
})
```
Update `web/src/App.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { fetchVersion, type PublicVersion } from './api/version'
import { Feed } from './components/Feed'
import { DailyView } from './components/DailyView'

type View = 'feed' | 'daily'

export default function App() {
  const [v, setV] = useState<PublicVersion | null>(null)
  const [view, setView] = useState<View>('feed')
  useEffect(() => {
    fetchVersion().then(setV).catch(() => {})
  }, [])
  return (
    <div className="app">
      <header className="app-header">
        <h1>AI HOT（内网）</h1>
        <p className="app-sub">AI 资讯精选</p>
        <nav className="app-nav">
          <button className={view === 'feed' ? 'active' : ''} onClick={() => setView('feed')}>资讯流</button>
          <button className={view === 'daily' ? 'active' : ''} onClick={() => setView('daily')}>日报</button>
        </nav>
      </header>
      <main>{view === 'feed' ? <Feed /> : <DailyView />}</main>
      <footer className="app-footer">
        {v && <span>API v{v.apiVersion} · Skill v{v.skillVersion}</span>}
      </footer>
    </div>
  )
}
```
Append to `web/src/index.css`:
```css
.app-nav { display: flex; gap: 8px; margin: 12px 0 4px; }
.app-nav button { padding: 6px 16px; border: 1px solid #ddd; border-radius: 6px; background: #fff; cursor: pointer; }
.app-nav button.active { background: #1a7f5a; color: #fff; border-color: #1a7f5a; }
.daily-date { color: #888; font-size: .85rem; margin-bottom: 12px; }
.daily-lead { border-left: 3px solid #1a7f5a; padding: 4px 14px; margin-bottom: 18px; }
.daily-lead h2 { margin: 0 0 6px; font-size: 1.2rem; }
.daily-lead p { color: #444; line-height: 1.6; }
.daily-section { margin-bottom: 20px; }
.daily-section h3 { font-size: 1rem; margin: 0 0 8px; }
.daily-item { margin-bottom: 12px; }
.daily-flash { margin-bottom: 6px; font-size: .9rem; }
.daily-status { color: #888; }
@media (prefers-color-scheme: dark) {
  .app-nav button { background: #1c1c1c; color: #ddd; border-color: #444; }
  .daily-lead p { color: #bbc; }
}
```

- [ ] **Step 4: Full suite + build** — `cd web && npx vitest run` → all PASS; `npm run build` → green.

- [ ] **Step 5: Commit**
```bash
git add web/src/api/daily.ts web/src/api/daily.test.ts web/src/components/DailyView.tsx web/src/components/DailyView.test.tsx web/src/App.tsx web/src/App.test.tsx web/src/index.css
git commit -m "feat(web): daily report view + nav toggle"
```

---

## Self-Review

- **Spec coverage:** Implements 工序⑦ (日报主编) per the design doc + openapi: window `[date 00:00Z, +24h)` over selected items (via the new `Until`), deterministic category sections in the contract's label order, LLM-written lead (nil-lead on failure — contract-legal), `dailies` storage, the three read endpoints with their exact validation rules (404 semantics, `^\d{4}-\d{2}-\d{2}$` → 400, take strict 1-180 default 30 → 400), a manual `cmd/gendaily` trigger, and a front-end 日报 view. Deliberately deferred (with a home): scheduled/cron generation (deployment wiring — gendaily is the unit it will call), flashes semantics refinement (v1 = most-recent-5 of the selected pool; aihot's real flash source is unknown), the dailies-archive front-end list (the API exists; the page shows latest only — small follow-up), ETag/304 on daily endpoints (contract doesn't require it there), and `/items/{id}` permalink pages (separate plan — daily links point at permalinks that 404 until then; same is already true of feed links).
- **Placeholder scan:** No TBD/TODO. Every step has complete code or an exact command. Task 5 Step 5 references the existing httpserv.go mount pattern rather than fabricating its current text (implementer must READ it) — honest, not a placeholder.
- **Type consistency:** `daily.Report/Section/SectionItem/Flash/Lead/Entry` json tags match the openapi wire names (`sourceUrl`, `sourceName`, `leadParagraph`, `generatedAt`, …). `Store` methods (`Upsert/GetLatest/GetByDate/ListRecent` + `ErrNotFound`) match the `publicapi.DailyStore` interface exactly. `buildSections/buildFlashes/generateLead/parseLead` defined in Task 3, used by Task 4's `Generator`; `LLM` iface satisfied by both `fakeLLM` (tests), `noLLM` (cmd) and `*llm.Client`. `items.ListParams.Until` (Task 1) is used by `Generator.Generate`. Front-end `DailyReport` mirrors the same wire shape; `DailyView` uses the existing `formatBeijingTime`. `writeError` reused from P1.4's handler file (same package).

## Follow-on

- **Cron/deployment wiring**: schedule `gendaily` daily + the ingest/pipeline loop on a VPN host.
- **Dailies archive page** (front-end list over `/api/public/dailies` + per-date view via `/api/public/daily/{date}`).
- **`/items/{id}` detail page** (feed + daily permalinks currently have no target page).
- **Flashes semantics** revisit (e.g. all-pool recent items) once mode=all policy lands.
