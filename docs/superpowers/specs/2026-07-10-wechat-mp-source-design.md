# 公众号 (WeChat MP) Content Source — Design

**Date:** 2026-07-10
**Status:** Draft — awaiting user review
**Goal:** Ingest AI-relevant 微信公众号 articles from the Mac-side `wechat-corpus`
into the Melos-hosted aihot feed, reusing the existing enrich → score → 精选/全部
pipeline.

---

## Decisions (confirm / veto)

1. **Content scope — AI-relevant only, coarse pre-filter before ingest.**
   *(User-selected.)* A cheap keyword gate runs before the expensive LLM enrich;
   the LLM relevance score remains the real 精选 gate.
2. **Topology — rsync markdown → Melos + a Melos-side source.**
   *(Chosen on the user's behalf while they were away; recommended.)* Keeps all
   ingestion logic self-contained on Melos (matching the 24/7 hosting principle);
   only a dumb file sync crosses machines. Alternative if rejected: a Mac-side
   command that `InsertRaw`s straight into the Melos Postgres over the SSH tunnel
   (matches the existing wechat-push "Mac 出料 / Melos 接收" pattern).

---

## Context

- **Corpus** (`/Users/didi/wechat-corpus/<公众号>/<YYYY-MM-DD>__<slug>__<hash>.md`):
  ~2040 articles across 10 大厂技术号, ~3-month window. Each file is YAML
  frontmatter (`id, mp_id, mp_name, title, url, publish_time, description,
  crawled_at, source`) + a `#`-title + markdown body.
- **Crawler**: `we-mp-rss` runs on the **Mac**; per HANDOFF the crawl pipeline is
  owned separately and **must not be modified** — this feature is read-only
  against the corpus.
- **aihot**: server + enrich cron run on **Melos** (10.190.12.242). `RawItem`
  already reserved `SourceKind` values like `"mp_hot"`, so MP articles flow
  through the same `raw_items → pipeline → items` path.
- **Runner limits**: `maxItemAge = 14d` (skips older items at fetch time —
  steady-state stays fresh); `batchSize=20, maxBatches=5` → ≤100 enrich
  calls/pulse (historical items drain ~100/cycle once in `raw_items`).

## Architecture

```
Mac corpus ──rsync -a --delete (cron)──▶ Melos /root/aihot/corpus/wechat/
                                                  │
                                     MPCorpusSource.Fetch()
                                  parse frontmatter → keyword pre-filter
                                                  │  []RawItem (SourceKind="mp")
                                     InsertRaw (dedup id=sha256(url))
                                                  │
                                     pulse enrich (LLM: 中文摘要/相关性/分类)
                                                  │
                                       relevance floor → 精选 / 全部
```

## Components

### 1. Sync (Mac → Melos)
Mac cron: `rsync -a --delete /Users/didi/wechat-corpus/ melos:/root/aihot/corpus/wechat/`.
Read-only mirror. Independent of the aihot server; a sync failure leaves the last
good mirror in place so the pulse keeps working. Documented as a scripts entry
(not baked into the Go binary).

### 2. `MPCorpusSource` — `internal/ingest/mpsource.go`
Implements `ingest.Source`:
- `Name() string` → a fixed label (e.g. `"WeChat MP"`); per-item `Source` is set
  to the article's `mp_name`.
- `Fetch(ctx) ([]RawItem, error)` → walk the corpus root, parse each `.md`,
  keyword-filter, map to `RawItem`.

**Frontmatter parsing** — a small manual scalar scanner over the block between the
first two `---` lines: capture top-level `key: value` pairs (`url`, `mp_name`,
`title`, `publish_time`), strip surrounding quotes, ignore YAML folded
continuation lines (the multi-line `description`, which we don't need — the full
body is the `RawContent`). **No new dependency.** A file missing any required
field (`url`, `title`) is skipped.

**RawItem mapping:**
| RawItem field | Source |
|---|---|
| `ID` | `RawID(url)` (sha256 of the weixin URL — stable, dedup key) |
| `Source` | `mp_name` |
| `SourceKind` | `"mp"` |
| `URL` | `url` (mp.weixin.qq.com/s/…) |
| `Title` | `title` (Chinese) |
| `PublishedAt` | `publish_time` (RFC3339 with +08:00) |
| `RawContent` | markdown body (after frontmatter) |
| `ImageURL` | best-effort: first `<img>`/`![]()` in body, else nil |
| `VideoURL` | nil (weixin embeds not resolvable) |

### 3. Pre-filter (keyword gate)
Case-insensitive substring match of `title + description` against a curated
AI-specific set:
`AI, 大模型, LLM, Agent, 智能体, AI Coding, Vibe Coding, Harness, Spec-Driven, SDD,
RAG, MCP, Claude, GPT, Copilot, 通义, prompt, 提示词, 推理模型, 多模态, 微调`.
Curated to avoid generic tech false-positives (e.g. not bare "模型"). The LLM
enrich is the second gate: low-relevance survivors get low scores and stay out of
精选. Keyword set lives in one place, table-tested.

### 4. Pulse wiring — `internal/pulse/sources.go` / `pulse.go`
A corpus-path config (env or app.toml, default `/root/aihot/corpus/wechat`) that,
when the dir exists, appends `MPCorpusSource` to the runner's sources. The
existing 14-day `MaxAge` keeps steady-state to fresh articles; `InsertRaw ON
CONFLICT DO NOTHING` makes each scan idempotent. When the path is unset/missing,
behavior is unchanged (RSS only).

### 5. `cmd/importmp` — one-off historical backfill (optional)
Reads the whole corpus, applies the same pre-filter, `InsertRaw`s all matching
articles **with no age gate**; the Melos pulse then drains + enriches ~100/cycle.
User-triggered (like `cmd/backfillimages`), safe to re-run (idempotent).

### 6. Enrich — unchanged
The LLM enricher already emits Chinese summaries; Chinese-in → Chinese-out. Title
stays Chinese (no `TitleEN`). Category + relevance scored as usual.

## Error Handling
- Unreadable / malformed `.md` → skip that file, continue (log count).
- Missing required frontmatter field → skip that item.
- Sync failure → last good mirror remains; pulse unaffected.
- All best-effort and per-item isolated, matching the existing runner semantics
  (one bad item never aborts a batch).

## Testing
`internal/ingest/mpsource_test.go`:
- Frontmatter parse: happy path, quoted `publish_time`, folded `description`
  ignored, CRLF/whitespace tolerance.
- Keyword filter: included terms match, generic-tech excluded, case-insensitive.
- Missing-field files skipped; empty dir → empty slice, no error.
- `id` stability: same URL → same `RawID`.
- Runner integration over a `t.TempDir()` corpus with 2–3 fixture `.md` files.

## File Structure
- `internal/ingest/mpsource.go` — `MPCorpusSource`, frontmatter scanner, keyword filter
- `internal/ingest/mpsource_test.go`
- `internal/pulse/sources.go` — corpus-path config → append source
- `cmd/importmp/main.go` — one-off historical backfill
- `scripts/sync-wechat-corpus.sh` (+ doc) — the Mac→Melos rsync (documented)

## Out of Scope (YAGNI)
- Modifying the `we-mp-rss` crawl pipeline.
- Video/rich-media extraction from weixin.
- Adding new 公众号 subscriptions (that's a corpus-side change).
- A source-type filter in the UI (`一手信源/资讯/推文`) — deferred, separate feature.
