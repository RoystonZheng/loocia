# Rich Detail Page (SSR) — Design

**Date:** 2026-07-13
**Status:** Approved (all decisions locked with user)
**Goal:** Turn the short SSR permalink page into a full reading page — sidebar shell,
精选理由 + AI 摘要 boxes, full 原文 body, 导出 Markdown — matching the reference layout.

## Decisions (locked)

1. **精选理由 box** — ADD a new LLM `reason_cn` field. **Backfill only selected items.**
2. **Page form** — keep **SSR + add a sidebar shell** (noindex/permalink/contract unchanged).
3. **原文 body** — **full text 直出** (reverses the earlier anti-scrape "no body" choice;
   internal noindex tool). MP body = Markdown → goldmark; RSS body = HTML → bluemonday.

## Layout (adapted to AI Cool)

```
┌──────────┬──────────────────────────────────────────────┐
│ AI Cool  │  IT之家（RSS）      [精选 ·77]  [导出 Markdown]│
│          │  # 大标题                                     │
│ 精选     │  2026-07-13 · 阅读原文·ithome.com             │
│ 全部AI   │  ┌ 精选理由 ─────────────────────────────┐   │
│ AI日报   │  └───────────────────────────────────────┘   │
│          │  ┌ AI 摘要 ──────────────────────────────┐   │
│ ☾ ▢ ☀   │  └───────────────────────────────────────┘   │
│          │  原文                                         │
│          │  <全文渲染…>                                  │
│          │  #行业动态   [阅读原文 →]                     │
└──────────┴──────────────────────────────────────────────┘
```

Sidebar mirrors the SPA's own nav (AI Cool logo + 精选/全部 AI 动态/AI 日报 → link to
`/`; theme toggle sharing the SPA's localStorage key so the choice persists across
SPA↔detail navigation). We replicate *our* SPA sidebar, not the reference's extra
nav items (主题/收藏/… which our product intentionally doesn't have).

## Components

### 1. LLM `reason_cn` field
- `pipeline/enrichment.go`: `Enrichment.ReasonCN string`; system prompt gains a
  `"reason_cn": 一句话说明为什么值得精选/关注` field; `parseEnrichment` reads it.
- `items/item.go`: `Item.Reason *string`; `items/schema.sql`: `reason TEXT` + idempotent
  ALTER; `items/write.go`: 22→23-column lockstep (itemColumns/listColumns/Upsert
  VALUES $23/ON CONFLICT SET/scanItem all gain `reason`).
- `pipeline/processor.go`: `toItem` sets `Reason` from enrichment (nil when empty).
- New items get it automatically; historical items have NULL until backfilled.

### 2. Body rendering
- New deps: `github.com/yuin/goldmark`, `github.com/microcosm-cc/bluemonday`.
- `detailpage/render.go`: `renderBody(sourceKind, body string) template.HTML`:
  - `mp` → goldmark Markdown→HTML → bluemonday UGCPolicy sanitize.
  - else (rss) → bluemonday UGCPolicy sanitize (RSS body is HTML/text).
  - Images get `loading="lazy"`; a tiny inline `onerror` hides broken (weixin hotlink)
    images — added by a post-process string replace on `<img `.
- Sanitize is mandatory (third-party HTML → XSS guard) even though noindex.

### 3. 导出 Markdown
- `detailpage` handler: when `?format=md`, respond `text/markdown; charset=utf-8`
  with `# title\n\n> source · date · url\n\n## 摘要\n{summary}\n\n## 原文\n{body}`.
  MP body is already Markdown; RSS HTML is passed through (best-effort — readable).
- The button links to `?format=md` with a `download` hint via `Content-Disposition`.

### 4. Detail template + sidebar
- `detailpage/page.go`: expand `viewModel` with Reason, Body (template.HTML),
  SourceKind, ExportURL, domain. Template grows the two-column shell + boxes.
- Inline `<style>` extends the existing cool-blue system; inline `<script>` for the
  theme toggle reading the SPA's localStorage key (verify key in `web/src/theme.ts`).

### 5. Reason backfill (selected only)
- `cmd/backfillreason`: SELECT selected items with `reason IS NULL`; for each, one
  focused LLM call over (title, summary, category) → one-sentence reason; `UPDATE
  items SET reason=$1`. Idempotent, read-mostly, safe to re-run. Uses the retrying
  `llm.Client`.

## Error handling
- Missing body → skip 原文 section. Missing reason/summary → skip that box.
- goldmark/bluemonday never error on our inputs; empty in → empty out.
- Export of a missing item → 404 (same visibility gate as the HTML page).

## Testing
- `enrichment_test.go`: reason parsed; prompt lists reason_cn.
- `render_test.go`: MP markdown → has `<p>`/`<h`; RSS `<script>` stripped; `<img`
  gets lazy+onerror; empty → empty.
- `detailpage handler_test.go`: body rendered; reason box present when set, absent
  when nil; sidebar nav present; `?format=md` returns text/markdown with title+body;
  cool-blue retained.
- `items` write/scan round-trips reason.

## File structure
- `pipeline/enrichment.go`, `pipeline/processor.go` — reason field
- `items/item.go`, `items/schema.sql`, `items/write.go`, `items/query.go` — reason column
- `detailpage/render.go` (+ test) — body rendering
- `detailpage/page.go`, `detailpage/handler.go` (+ tests) — template, sidebar, export
- `cmd/backfillreason/main.go` — selected-item reason backfill

## Out of scope
- Making the SPA sidebar nav items deep-link into views (SPA doesn't read URL params).
- 收藏/主题/Agent接入/关于/更新日志/反馈 nav items (not in our product).
- Full-body backfill of reason for non-selected items.
