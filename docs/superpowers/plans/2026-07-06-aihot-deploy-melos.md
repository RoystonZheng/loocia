# Deployment Wiring (Melos cron) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the system run itself: a `cmd/pulse` unit that ingests RSS and drains the enrichment queue, deploy scripts that cross-compile and install pulse/gendaily/hotpass on Melos with crontab schedules against the production `aihot` DB, and a live verification that real RSS flows end-to-end into the browser.

**Architecture:** Probed 2026-07-06: Melos (Debian 13, x86_64) reaches openai.com / arxiv / blog.google directly, reaches the intranet LLM proxy (401), has local PG and crontab/systemd — so the WHOLE pipeline runs on Melos (no Mac dependency once deployed; the LLM key already lives on Melos in `/root/wechat-push/.env`). Mac cross-compiles static linux binaries (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64` — pgx is pure Go) and scp's them. Production data lives in the `aihot` DB (tests keep `aihot_test`; the SSH tunnel serves both — same port, different db name). A new `internal/pulse` package owns the only new logic: the default source list / JSON override loader and the bounded drain loop (ingest once → ProcessBatch(20) up to 5 batches, stopping early when a batch makes no progress so failing items can't spin). Cron: pulse every 30 min, hotpass hourly at :20, gendaily hourly at :10 (upsert keeps "today's daily" fresh all day). Logs append to `/root/aihot/log/*.log`.

**Tech Stack:** Go (`internal/pulse`, `cmd/pulse`), bash deploy scripts, crontab; existing ingest/pipeline/daily/cluster packages.

---

## Environment / how to run

- Go via `GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org` (or `make build`/`make test` with `-p 1`).
- Live PG tunnel on localhost:5432 (restart: `ssh -N -L 5432:localhost:5432 melos &`). Test DSN `...aihot_test`; **production DSN `postgres://aihot:aihot@localhost:5432/aihot`** (via tunnel from Mac; `localhost:5432/aihot` natively on Melos).
- LLM key (SECRET): on Melos at `/root/wechat-push/.env` (`LLM_API_KEY`); the cron wrapper exports it as `AIHOT_LLM_API_KEY`. On Mac only for gated tests (inline export as before).
- Melos paths: install root `/root/aihot/` (`bin/`, `etc/`, `log/`).

## Default sources (verified reachable FROM MELOS 2026-07-06)

- OpenAI Blog — `https://openai.com/news/rss.xml`
- Google AI Blog — `https://blog.google/technology/ai/rss/`
(arXiv cs.AI works too but its daily volume would burn LLM enrichment calls — documented in the example JSON, not in defaults. huggingface.co times out from Melos — excluded.)

## File Structure

- `server/internal/pulse/sources.go` — `SourceConfig`, `LoadSources` (defaults / JSON file).
- `server/internal/pulse/pulse.go` — `Deps`, `Summary`, `Run` (ensure schemas → ingest → bounded drain).
- `server/internal/pulse/*_test.go` — pure config tests + live-DB wiring test (httptest RSS + fake LLM).
- `server/cmd/pulse/main.go` — thin main.
- `deploy/build-linux.sh` — cross-compile the three binaries into `deploy/out/`.
- `deploy/aihot-cron.sh` — the wrapper cron invokes (env + key sourcing + exec + log).
- `deploy/install-melos.sh` — scp binaries+wrapper, create dirs, install crontab.
- `deploy/README.md` — runbook (schedules, logs, manual runs, rollback).

---

### Task 1: `internal/pulse` + `cmd/pulse`

**Files:**
- Create: `server/internal/pulse/sources.go`, `server/internal/pulse/pulse.go`
- Test: `server/internal/pulse/sources_test.go`, `server/internal/pulse/pulse_test.go`
- Create: `server/cmd/pulse/main.go`

- [ ] **Step 1: Write the failing tests**

`server/internal/pulse/sources_test.go`:
```go
package pulse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSourcesDefaults(t *testing.T) {
	srcs, err := LoadSources("")
	if err != nil {
		t.Fatalf("LoadSources(\"\"): %v", err)
	}
	if len(srcs) < 2 {
		t.Fatalf("want >=2 default sources, got %d", len(srcs))
	}
	names := map[string]bool{}
	for _, s := range srcs {
		names[s.Name()] = true
	}
	if !names["OpenAI Blog"] || !names["Google AI Blog"] {
		t.Fatalf("default names: %v", names)
	}
}

func TestLoadSourcesFromJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sources.json")
	content := `[{"name":"Feed A","url":"https://a.example/rss"},{"name":"Feed B","url":"https://b.example/rss"}]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	srcs, err := LoadSources(path)
	if err != nil {
		t.Fatalf("LoadSources(json): %v", err)
	}
	if len(srcs) != 2 || srcs[0].Name() != "Feed A" || srcs[1].Name() != "Feed B" {
		t.Fatalf("srcs: %d %v", len(srcs), srcs)
	}
}

func TestLoadSourcesRejectsBadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(path, []byte(`{not json`), 0o644)
	if _, err := LoadSources(path); err == nil {
		t.Fatal("bad json should error")
	}
	if _, err := LoadSources(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file should error")
	}
	// Entries missing name/url are rejected.
	path2 := filepath.Join(t.TempDir(), "empty.json")
	_ = os.WriteFile(path2, []byte(`[{"name":"","url":""}]`), 0o644)
	if _, err := LoadSources(path2); err == nil {
		t.Fatal("empty name/url should error")
	}
}
```

`server/internal/pulse/pulse_test.go`:
```go
package pulse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"aihot-server/internal/db"
	"aihot-server/internal/ingest"

	"github.com/jackc/pgx/v5/pgxpool"
)

const pulseRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>T</title>
  <item><title>Pulse Item One</title><link>https://ex.com/pulse-1</link>
    <description>body one</description><pubDate>Mon, 06 Jul 2026 08:00:00 GMT</pubDate></item>
  <item><title>Pulse Item Two</title><link>https://ex.com/pulse-2</link>
    <description>body two</description><pubDate>Mon, 06 Jul 2026 09:00:00 GMT</pubDate></item>
</channel></rss>`

type fakeLLM struct{ reply string }

func (f fakeLLM) Complete(ctx context.Context, system, user string) (string, error) {
	return f.reply, nil
}

func testPool(t *testing.T) *pgxpool.Pool {
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
	for _, tbl := range []string{"raw_items", "items"} {
		// Tables may not exist on a fresh DB; Run ensures schemas, so ignore errors here.
		_, _ = pool.Exec(context.Background(), "TRUNCATE "+tbl)
	}
	return pool
}

func TestRunIngestsAndEnriches(t *testing.T) {
	pool := testPool(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(pulseRSS))
	}))
	defer srv.Close()

	sum, err := Run(context.Background(), Deps{
		Pool:    pool,
		LLM:     fakeLLM{reply: `{"title_cn":"中文标题","summary_cn":"摘要","category":"ai-models","relevance":90,"score":80,"selected":true}`},
		Sources: []ingest.Source{ingest.NewRSSSource("Test Feed", srv.URL)},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Inserted != 2 || sum.Processed != 2 || sum.Failed != 0 {
		t.Fatalf("summary: %+v", sum)
	}

	// Items landed enriched.
	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM items WHERE title='中文标题'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("enriched items: %d", count)
	}

	// Second run: idempotent — nothing new inserted or processed.
	sum2, err := Run(context.Background(), Deps{
		Pool:    pool,
		LLM:     fakeLLM{reply: `{}`},
		Sources: []ingest.Source{ingest.NewRSSSource("Test Feed", srv.URL)},
	})
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if sum2.Inserted != 0 || sum2.Processed != 0 {
		t.Fatalf("second run should be a no-op: %+v", sum2)
	}
}

func TestRunStopsWhenNoProgress(t *testing.T) {
	pool := testPool(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(pulseRSS))
	}))
	defer srv.Close()

	// LLM returns garbage → every enrichment fails → items stay unprocessed.
	// The drain loop must stop after the first no-progress batch, not spin to the cap.
	sum, err := Run(context.Background(), Deps{
		Pool:    pool,
		LLM:     fakeLLM{reply: `garbage not json`},
		Sources: []ingest.Source{ingest.NewRSSSource("Test Feed", srv.URL)},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Processed != 0 || sum.Failed != 2 || sum.Batches != 1 {
		t.Fatalf("no-progress summary: %+v", sum)
	}
}
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org AIHOT_TEST_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' go test ./internal/pulse/ -v` → FAIL (`undefined: LoadSources` / `Run`).

- [ ] **Step 3: Implement**

`server/internal/pulse/sources.go`:
```go
package pulse

import (
	"encoding/json"
	"fmt"
	"os"

	"aihot-server/internal/ingest"
)

// SourceConfig is one feed entry in the optional sources JSON file.
type SourceConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// defaultSources are feeds verified reachable from the deploy host (Melos).
// Volume-heavy feeds (e.g. arXiv) are deliberately not defaults — each new item
// costs one LLM enrichment call. Override with a JSON file when needed.
var defaultSources = []SourceConfig{
	{Name: "OpenAI Blog", URL: "https://openai.com/news/rss.xml"},
	{Name: "Google AI Blog", URL: "https://blog.google/technology/ai/rss/"},
}

// LoadSources returns the RSS sources: the embedded defaults when path is
// empty, else the JSON file at path ([{"name":...,"url":...}]).
func LoadSources(path string) ([]ingest.Source, error) {
	configs := defaultSources
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read sources file: %w", err)
		}
		configs = nil
		if err := json.Unmarshal(data, &configs); err != nil {
			return nil, fmt.Errorf("parse sources file: %w", err)
		}
	}
	var out []ingest.Source
	for i, c := range configs {
		if c.Name == "" || c.URL == "" {
			return nil, fmt.Errorf("source %d: name and url are required", i)
		}
		out = append(out, ingest.NewRSSSource(c.Name, c.URL))
	}
	return out, nil
}
```

`server/internal/pulse/pulse.go`:
```go
package pulse

import (
	"context"
	"fmt"

	"aihot-server/internal/ingest"
	"aihot-server/internal/items"
	"aihot-server/internal/pipeline"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Deps are the pulse unit's dependencies.
type Deps struct {
	Pool    *pgxpool.Pool
	LLM     pipeline.LLM
	Sources []ingest.Source
}

// Summary reports one pulse run.
type Summary struct {
	Fetched      int
	Inserted     int
	Skipped      int
	SourceErrors int
	Processed    int
	Failed       int
	Batches      int
}

const (
	batchSize  = 20
	maxBatches = 5 // hard cap: ≤100 enrichment calls per pulse
)

// Run executes one pulse: ensure schemas → ingest all sources → drain the
// enrichment queue in bounded batches. A batch that makes no progress
// (only failures) stops the drain so poison items can't spin the loop.
func Run(ctx context.Context, d Deps) (Summary, error) {
	var sum Summary

	rawStore := ingest.NewRawStore(d.Pool)
	if err := rawStore.EnsureSchema(ctx); err != nil {
		return sum, fmt.Errorf("raw schema: %w", err)
	}
	itemsStore := items.New(d.Pool)
	if err := itemsStore.EnsureSchema(ctx); err != nil {
		return sum, fmt.Errorf("items schema: %w", err)
	}

	res, err := ingest.NewRunner(rawStore, d.Sources...).RunOnce(ctx)
	if err != nil {
		return sum, fmt.Errorf("ingest: %w", err)
	}
	sum.Fetched, sum.Inserted, sum.Skipped = res.Fetched, res.Inserted, res.Skipped
	sum.SourceErrors = len(res.Errors)

	proc := pipeline.NewProcessor(rawStore, itemsStore, pipeline.NewEnricher(d.LLM))
	for sum.Batches < maxBatches {
		batch, err := proc.ProcessBatch(ctx, batchSize)
		if err != nil {
			return sum, fmt.Errorf("enrich batch: %w", err)
		}
		if batch.Processed+batch.Failed == 0 {
			break // queue drained
		}
		sum.Batches++
		sum.Processed += batch.Processed
		sum.Failed += batch.Failed
		if batch.Processed == 0 {
			break // no progress: only failures — stop, retry next pulse
		}
	}
	return sum, nil
}
```

`server/cmd/pulse/main.go`:
```go
// pulse runs one ingest+enrich cycle: fetch all RSS sources, then drain the
// enrichment queue in bounded batches.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... \
//	  go run ./cmd/pulse [-sources /path/sources.json]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/internal/db"
	"aihot-server/internal/llm"
	"aihot-server/internal/pulse"
)

func main() {
	sourcesPath := flag.String("sources", "", "optional JSON sources file (default: embedded list)")
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
	sources, err := pulse.LoadSources(*sourcesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sources:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	sum, err := pulse.Run(ctx, pulse.Deps{Pool: pool, LLM: client, Sources: sources})
	if err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
	fmt.Printf("pulse: fetched=%d inserted=%d skipped=%d srcerrs=%d enriched=%d failed=%d batches=%d\n",
		sum.Fetched, sum.Inserted, sum.Skipped, sum.SourceErrors, sum.Processed, sum.Failed, sum.Batches)
}
```

- [ ] **Step 4: Run to confirm PASS** — same command → all 5 PASS. `make build` → green (compiles cmd/pulse).

- [ ] **Step 5: Commit**
```bash
git add server/internal/pulse/ server/cmd/pulse/
git commit -m "feat(pulse): ingest+enrich pulse unit + command"
```

---

### Task 2: Deploy scripts + runbook

**Files:**
- Create: `deploy/build-linux.sh`, `deploy/aihot-cron.sh`, `deploy/install-melos.sh`, `deploy/README.md`

- [ ] **Step 1: Write the scripts**

`deploy/build-linux.sh`:
```bash
#!/usr/bin/env bash
# Cross-compile the pipeline binaries for Melos (linux/amd64, static).
set -euo pipefail
cd "$(dirname "$0")/../server"
OUT="../deploy/out"
mkdir -p "$OUT"
export GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64
for cmd in pulse gendaily hotpass; do
  echo "building $cmd..."
  go build -o "$OUT/$cmd" "./cmd/$cmd"
done
echo "built: $(ls -1 "$OUT")"
```

`deploy/aihot-cron.sh` (installed on Melos at `/root/aihot/bin/aihot-cron.sh`; cron calls it with the unit name):
```bash
#!/usr/bin/env bash
# Cron wrapper: env + LLM key + exec one pipeline unit, logging to /root/aihot/log.
#   aihot-cron.sh pulse|gendaily|hotpass [extra args...]
set -euo pipefail
UNIT="$1"; shift || true

export AIHOT_DATABASE_URL="postgres://aihot:aihot@localhost:5432/aihot"
# The LLM key lives in the wechat-push env file on this host.
export AIHOT_LLM_API_KEY="$(grep '^LLM_API_KEY' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"

LOG="/root/aihot/log/${UNIT}.log"
{
  echo "--- $(date -u '+%F %T') UTC ${UNIT} start"
  "/root/aihot/bin/${UNIT}" "$@" 2>&1
  echo "--- $(date -u '+%F %T') UTC ${UNIT} done rc=$?"
} >> "$LOG" 2>&1
```

`deploy/install-melos.sh`:
```bash
#!/usr/bin/env bash
# Install/refresh the pipeline on Melos: binaries + wrapper + crontab.
set -euo pipefail
cd "$(dirname "$0")"
HOST="${1:-melos}"

[ -x out/pulse ] || { echo "run build-linux.sh first"; exit 1; }

ssh "$HOST" 'mkdir -p /root/aihot/bin /root/aihot/etc /root/aihot/log'
scp out/pulse out/gendaily out/hotpass aihot-cron.sh "$HOST:/root/aihot/bin/"
ssh "$HOST" 'chmod +x /root/aihot/bin/*'

# Crontab: keep other entries, replace the aihot block idempotently.
ssh "$HOST" '
  (crontab -l 2>/dev/null | sed "/# aihot-begin/,/# aihot-end/d"
   echo "# aihot-begin"
   echo "*/30 * * * * /root/aihot/bin/aihot-cron.sh pulse"
   echo "10 * * * * /root/aihot/bin/aihot-cron.sh gendaily"
   echo "20 * * * * /root/aihot/bin/aihot-cron.sh hotpass"
   echo "# aihot-end") | crontab -
  crontab -l | sed -n "/# aihot-begin/,/# aihot-end/p"
'
echo "installed."
```

`deploy/README.md`:
```markdown
# aihot Melos 部署 runbook

## 架构
全管线跑在 Melos(`ssh melos`,Debian13/x86_64):本机 PG 生产库 `aihot`
(测试库 `aihot_test` 不受影响),LLM key 复用 `/root/wechat-push/.env`。
Mac 只负责交叉编译 + scp。

## 定时(crontab,标记块 `# aihot-begin/end`)
| 单元 | 频率 | 作用 |
|---|---|---|
| pulse    | 每 30 分钟 | RSS 采集 + LLM 加工(≤100 条/次) |
| gendaily | 每小时 :10 | 重生成当天(UTC)日报,upsert 幂等 |
| hotpass  | 每小时 :20 | 聚类 + 热度重算(72h 窗口) |

## 发布 / 更新
    ./deploy/build-linux.sh     # Mac 交叉编译 → deploy/out/
    ./deploy/install-melos.sh   # scp + 刷新 crontab(幂等)

## 手动跑一次 / 看日志(在 Melos 上)
    /root/aihot/bin/aihot-cron.sh pulse
    tail -50 /root/aihot/log/pulse.log

## Mac 上看真实数据
    ssh -N -L 5432:localhost:5432 melos &        # 隧道
    cd server && AIHOT_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot' make run &
    cd web && npm run dev                        # localhost:5173

## 回滚 / 停用
    ssh melos 'crontab -l | sed "/# aihot-begin/,/# aihot-end/d" | crontab -'

## 已知边界
- 源清单默认 OpenAI Blog + Google AI Blog(内嵌);加源:上传 JSON 到
  /root/aihot/etc/sources.json 并把 pulse 的 cron 行改成
  `... pulse -sources /root/aihot/etc/sources.json`。arXiv 量大费 LLM,慎加。
- 失败条目留在 raw_items 未处理态,下轮 pulse 自动重试;毒条目不会死循环
  (无进展即停批)。
- LLM key 轮换后无需改动(每次执行时从 .env 现读)。
```

- [ ] **Step 2: Verify the scripts locally** — `chmod +x deploy/*.sh && bash -n deploy/build-linux.sh deploy/aihot-cron.sh deploy/install-melos.sh` (syntax check) then actually run `./deploy/build-linux.sh` → three linux binaries in `deploy/out/` (`file deploy/out/pulse` → ELF 64-bit x86-64). Add `deploy/out/` to `.gitignore`.

- [ ] **Step 3: Commit**
```bash
git add deploy/ .gitignore
git commit -m "feat(deploy): melos cross-compile + cron install scripts + runbook"
```

---

### Task 3: Deploy + live verification (the point)

No new code — execute the deployment and prove the system runs itself.

- [ ] **Step 1: Build + install** — `./deploy/build-linux.sh && ./deploy/install-melos.sh` → crontab block printed (3 entries).

- [ ] **Step 2: First real pulse on Melos** — `ssh melos '/root/aihot/bin/aihot-cron.sh pulse && tail -5 /root/aihot/log/pulse.log'` → the summary line with real fetched/inserted/enriched counts (OpenAI + Google feeds; first run may enrich up to 100 items — this is real LLM spend, expected). If a source errors (feed moved), that's isolated — note it.

- [ ] **Step 3: gendaily + hotpass on Melos** — `ssh melos '/root/aihot/bin/aihot-cron.sh gendaily && /root/aihot/bin/aihot-cron.sh hotpass && tail -3 /root/aihot/log/gendaily.log /root/aihot/log/hotpass.log'` → daily line (needs ≥1 selected item in today's UTC window — if none yet, `no selected items` is honest, rerun after more pulses) + hotpass line.

- [ ] **Step 4: Verify production DB** — `ssh melos "psql 'postgres://aihot:aihot@localhost:5432/aihot' -tAc \"SELECT count(*) FROM items; SELECT count(*) FROM raw_items WHERE processed; SELECT date, lead_title IS NOT NULL FROM dailies ORDER BY date DESC LIMIT 1;\""` → real counts.

- [ ] **Step 5: See REAL data in the browser (Mac)** — tunnel up, then `cd server && AIHOT_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot' make run &` + `cd web && npm run dev &`; Playwright :5173 → feed shows real enriched Chinese items from OpenAI/Google feeds; 日报 tab shows the generated daily (if any); screenshot. Kill both after.

- [ ] **Step 6: Confirm cron fires unattended** — check `ssh melos 'crontab -l | sed -n "/aihot-begin/,/aihot-end/p"'` and (if a :10/:20/:30 boundary passed during this task) `ssh melos 'ls -la /root/aihot/log/ && tail -2 /root/aihot/log/*.log'` for a cron-initiated entry. If no boundary passed, verifying the wrapper ran manually + crontab installed is the gate; note the first scheduled fire remains to be observed.

- [ ] **Step 7: Update the project docs** — append a short "生产部署" section to `docs/runbooks/README.md` pointing at `deploy/README.md`. Commit:
```bash
git add docs/runbooks/
git commit -m "docs: link melos deployment runbook"
```

---

## Self-Review

- **Spec coverage:** Delivers the design doc's 部署接线: scheduled 采集→加工 (pulse), 日报生成 (gendaily hourly, upsert-fresh), 聚类+热度定时重算 (hotpass hourly) — all on a VPN-independent intranet host with the production `aihot` DB, real verified sources, key sourced on-host, idempotent crontab install, logs, runbook, rollback. Deliberately deferred (with a home): serving the API/web from Melos (server binary + static hosting — a follow-on; Mac serves on demand via tunnel per the runbook), 代理重试/限流 + nginx 缓存 (hardening once volume is known), systemd timers instead of cron (cron suffices; documented), sources table in DB (JSON file override suffices for v1), monitoring/alerting (log files for now).
- **Placeholder scan:** No TBD/TODO. Scripts are complete and syntax-checkable; Task 3 steps are exact commands. The "first scheduled fire" caveat in T3 Step 6 is an honest observation boundary, not a placeholder.
- **Type consistency:** `pulse.Deps{Pool, LLM pipeline.LLM, Sources []ingest.Source}` — `pipeline.LLM` is the existing interface (satisfied by `*llm.Client` and the test's `fakeLLM`); `ingest.NewRSSSource` returns `*RSSSource` which satisfies `ingest.Source`; `pulse.Run` uses only existing, tested APIs (`RawStore.EnsureSchema`, `items.New/EnsureSchema`, `ingest.NewRunner(...).RunOnce`, `pipeline.NewProcessor(...).ProcessBatch`). `LoadSources` returns `[]ingest.Source`. The drain-loop stop conditions match the test assertions (`Batches==1` when all-fail; queue-drained breaks WITHOUT incrementing Batches beyond work done).
- **Cost note:** each pulse enriches ≤100 items (batch 20 × cap 5); defaults are two low-volume feeds; gendaily/hotpass are 1 LLM call each per hour. Bounded spend by construction.

## Follow-on

- **Serve API+web from Melos** (server binary + built frontend static hosting) — the UI alive 24/7.
- **Monitoring**: cron failure alerting (e.g. push to D-Chat bot), log rotation.
- **代理重试/限流, nginx 缓存** once real volume observed; source list expansion (arXiv with a volume budget).
