# 评分体系重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 0-100 `score`/`relevance` with 1-5 五档 (displayed S/A/B/C/D with tier colors), make 精选 = score≥3, drop the LLM `selected` bool, and migrate existing data by bucketing.

**Architecture:** Enrichment prompt asks for 1-5 score/relevance under an explicit value model. `parseEnrichment` clamps to [1,5]. `toItem` derives `selected = score≥3`, `ai_selected = score≥4`. Schema CHECK tightens to 1-5. Frontend maps 1-5 → letter + `data-tier` colored badge (React `ItemCard` + SSR `detailpage`). A one-time SQL migration buckets old 0-100 values and recomputes `selected` before the CHECK is tightened on production.

**Tech Stack:** Go (nuwa), PostgreSQL 17 (pgx), React + TypeScript (Vite, vitest, @testing-library/react with `fireEvent`).

**Build/deploy invariants (per project rules):**
- Go builds: prefix every `go`/`make` command with `GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org`. `make test` uses `-p 1`.
- Cross-compile for Melos: add `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`.
- ssh to Melos: `ssh -o ClearAllForwardings=yes melos` (existing tunnel conflicts).
- Commits: `git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit`, message ends with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
- Production DB `aihot`; test DB `aihot_test`. Never truncate production.

---

## File Structure

| File | Change |
|---|---|
| `server/internal/pipeline/enrichment.go` | Enrichment struct (drop `Selected`), rewrite `enrichSystemPrompt`, clamp in `parseEnrichment` |
| `server/internal/pipeline/enrichment_test.go` | Update tests: drop `selected`, new band assertions, clamp instead of reject |
| `server/internal/pipeline/processor.go` | `selected=score≥3`, `aiSelected=score≥4`, remove `selectionRelevanceFloor` |
| `server/internal/pipeline/processor_test.go` | Rewrite `TestToItemSelectionFloor` → score-based selection; fix fixtures |
| `server/internal/items/schema.sql` | `score` + `ai_relevance` CHECK → `BETWEEN 1 AND 5` |
| `web/src/format.ts` | Add `scoreLabel(n)` + `scoreTier(n)` |
| `web/src/format.test.ts` | Tests for `scoreLabel`/`scoreTier` |
| `web/src/components/ItemCard.tsx` | Badge shows letter + `data-tier` |
| `web/src/components/ItemCard.test.tsx` | Update score assertions to letters + tier |
| `web/src/index.css` | `.badge-score[data-tier=...]` tier colors (light+dark) |
| `server/internal/detailpage/page.go` | viewModel score→letter + `Tier`, template `data-tier`, embedded CSS tier colors |
| `server/internal/detailpage/page_test.go` | Assert letter + tier in rendered page (if such a test exists; else add) |
| _(deployment)_ | One-time migration SQL on `aihot`, rebuild+ship server/pulse/web |

---

## Task 1: Enrichment prompt + struct + clamp (Go)

**Files:**
- Modify: `server/internal/pipeline/enrichment.go`
- Test: `server/internal/pipeline/enrichment_test.go`

- [ ] **Step 1: Update the tests first (they encode the new contract)**

In `server/internal/pipeline/enrichment_test.go`, replace the JSON fixtures and band/range tests. Concretely:

Change `TestParseEnrichmentPlainJSON` (drop `selected`, use 1-5):
```go
func TestParseEnrichmentPlainJSON(t *testing.T) {
	raw := `{"title_cn":"模型X发布","summary_cn":"简短摘要。","category":"ai-models","relevance":5,"score":4,"reason_cn":"看点"}`
	e, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment: %v", err)
	}
	if e.TitleCN != "模型X发布" || e.SummaryCN != "简短摘要。" {
		t.Fatalf("title/summary: %+v", e)
	}
	if e.Category != "ai-models" || e.Relevance != 5 || e.Score != 4 {
		t.Fatalf("fields: %+v", e)
	}
}
```

Change `TestParseEnrichmentStripsCodeFence` fixture (drop `selected`, 1-5):
```go
	raw := "```json\n{\"title_cn\":\"标题\",\"summary_cn\":\"摘要\",\"category\":\"paper\",\"relevance\":3,\"score\":3}\n```"
```
and remove the `e.Selected` assertion — keep only the category check:
```go
	if e.Category != "paper" {
		t.Fatalf("fields: %+v", e)
	}
```

Change `TestParseEnrichmentRejectsBadCategory` fixture to 1-5 (`"relevance":3,"score":3`); assertion unchanged.

Replace `TestParseEnrichmentRejectsOutOfRangeScore` with a clamp test:
```go
func TestParseEnrichmentClampsScoreAndRelevance(t *testing.T) {
	// The model occasionally emits an old-scale or out-of-band number; clamp
	// into [1,5] instead of dropping the item.
	raw := `{"title_cn":"t","summary_cn":"s","category":"tip","relevance":90,"score":0}`
	e, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment should not fail on out-of-range: %v", err)
	}
	if e.Score != 1 || e.Relevance != 5 {
		t.Fatalf("clamp: score=%d relevance=%d, want score=1 relevance=5", e.Score, e.Relevance)
	}
}
```

Replace `TestSystemPromptHasScoreBands` with the five-tier + value-model check:
```go
func TestSystemPromptHasScoreTiers(t *testing.T) {
	// Five ordinal tiers, not the old 0-100 bands.
	for _, tier := range []string{"5", "4", "3", "2", "1"} {
		if !strings.Contains(enrichSystemPrompt, tier) {
			t.Fatalf("score rubric missing tier %q", tier)
		}
	}
	for _, kw := range []string{"工程", "行业", "深度", "营销"} {
		if !strings.Contains(enrichSystemPrompt, kw) {
			t.Fatalf("value model missing keyword %q", kw)
		}
	}
	if strings.Contains(enrichSystemPrompt, "selected") {
		t.Fatal("prompt should no longer ask for a selected field")
	}
}
```

Change `TestParseEnrichmentReadsReason` fixture (drop `selected`, 1-5): `"relevance":5,"score":4,"reason_cn":"首个可复用的实战范式"`.

Update `TestParseEnrichmentPlainJSON`-style fixtures anywhere else in the file that carry `"selected"` or 0-100 numbers.

- [ ] **Step 2: Run the tests, expect failures**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -run 'Enrichment|SystemPrompt' -v`
Expected: FAIL — `Selected` field still exists / prompt lacks tiers / clamp not implemented.

- [ ] **Step 3: Update the Enrichment struct**

In `server/internal/pipeline/enrichment.go`, remove the `Selected` field:
```go
type Enrichment struct {
	TitleCN   string `json:"title_cn"`
	SummaryCN string `json:"summary_cn"`
	Category  string `json:"category"`
	Relevance int    `json:"relevance"`
	Score     int    `json:"score"`
	ReasonCN  string `json:"reason_cn"`
}
```

- [ ] **Step 4: Rewrite the system prompt**

Replace the `enrichSystemPrompt` const body with:
```go
const enrichSystemPrompt = `你是 AI 资讯编辑。给定一条资讯的原始标题和正文，输出一个 JSON 对象（只输出 JSON，不要任何解释或代码块外的文字），字段：
- "title_cn": 简洁的中文标题
- "summary_cn": 2-3 句中文摘要
- "category": 必须是以下之一：ai-models、ai-products、industry、paper、tip
- "relevance": 整数 1-5，与「AI/大模型」主题的相关度：5=核心 AI、3=沾边、1=几乎无关
- "score": 整数 1-5，值得阅读的程度。判分内核 = （工程可操作 或 行业重要）×（深度/证据）：
    工程可操作 = 有方法/代码/benchmark/踩坑，能照着用或学；
    行业重要 = 头部玩家、格局变化、战略或投资信号、能力边界；
    深度/证据 = 讲清机制、有实测数据（浅的工程贴和浅的行业吹都要压低）。
    档位：
    5 = 重大突破/必读：一条腿打满且深度证据强（头部重大发布并讲透机制或有硬数据，或范式级工程实践带完整方法与 benchmark）；
    4 = 高价值：一条腿强且有实质深度（重要模型/产品更新且有分析，或扎实的工程实践或研究）；
    3 = 值得一看：相关且有信息量，但深度一般或影响有限；
    2 = 一般：边缘、增量很小、偏宣发、浅尝；
    1 = 低：营销通稿、标题党无实质、活动/直播/招聘/预告等通知、与 AI 弱相关。
    规则：工程腿和行业腿平权，两者都能到 5；看实质内容而非标题措辞；
    旧闻/不新颖/人尽皆知只要深且有用不扣分；压低营销宣发通知、标题党、与 AI 弱相关。
- "reason_cn": 一句话（不超过40字）说明这条为什么值得关注（点出关键看点，不要复述标题）

重要：title_cn、summary_cn、reason_cn 的文本内容里绝对不要出现英文双引号 " ；需要引用词语或名称时，一律改用中文引号「」或书名号《》。字符串值内出现未转义的英文双引号会破坏 JSON。`
```

- [ ] **Step 5: Clamp in parseEnrichment**

Replace the range-reject block (currently lines ~89-91):
```go
	if e.Relevance < 0 || e.Relevance > 100 || e.Score < 0 || e.Score > 100 {
		return e, fmt.Errorf("score/relevance out of range: rel=%d score=%d", e.Relevance, e.Score)
	}
```
with a clamp helper call:
```go
	e.Score = clampTier(e.Score)
	e.Relevance = clampTier(e.Relevance)
```
and add the helper near the bottom of the file:
```go
// clampTier bounds a 1-5 ordinal; out-of-range model output is clamped rather
// than rejected, so a stray 0/6/old-scale value never drops the whole item.
func clampTier(n int) int {
	if n < 1 {
		return 1
	}
	if n > 5 {
		return 5
	}
	return n
}
```

- [ ] **Step 6: Run the tests, expect pass**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -run 'Enrichment|SystemPrompt' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add server/internal/pipeline/enrichment.go server/internal/pipeline/enrichment_test.go
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(scoring): 1-5 tier prompt, drop selected, clamp score/relevance

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Score-based selection (Go)

**Files:**
- Modify: `server/internal/pipeline/processor.go`
- Test: `server/internal/pipeline/processor_test.go`

- [ ] **Step 1: Rewrite the selection test**

In `server/internal/pipeline/processor_test.go`, replace `TestToItemSelectionFloor` with:
```go
func TestToItemSelectionByScore(t *testing.T) {
	raw := ingest.RawItem{ID: "r1", URL: "https://x/1", Source: "S", Title: "Orig"}
	cases := []struct {
		name        string
		score       int
		wantSel     bool
		wantAISel   bool
	}{
		{"S is selected + strong", 5, true, true},
		{"A is selected + strong", 4, true, true},
		{"B is selected, not strong", 3, true, false},
		{"C is not selected", 2, false, false},
		{"D is not selected", 1, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Enrichment{TitleCN: "标题", SummaryCN: "摘要", Category: items.CategoryAIModels, Relevance: 4, Score: c.score}
			it := toItem(raw, e)
			if it.Selected != c.wantSel {
				t.Fatalf("Selected: got %v want %v (score=%d)", it.Selected, c.wantSel, c.score)
			}
			// ai_selected now records the "strong pick" signal (score>=4).
			if it.AISelected == nil || *it.AISelected != c.wantAISel {
				t.Fatalf("AISelected: got %v want %v (score=%d)", it.AISelected, c.wantAISel, c.score)
			}
			if it.AIRelevance == nil || *it.AIRelevance != 4 {
				t.Fatalf("AIRelevance: %v", it.AIRelevance)
			}
		})
	}
}
```

Also fix the `TestProcessBatchEnrichesAndMarksProcessed` fixture (line ~61-64) to 1-5 and drop `Selected`:
```go
	enr := fakeEnricher{out: Enrichment{
		TitleCN: "中文标题", SummaryCN: "中文摘要", Category: "ai-models",
		Relevance: 5, Score: 4,
	}}
```
and its assertions (line ~85, ~88):
```go
	if got.Category == nil || *got.Category != "ai-models" || got.Score == nil || *got.Score != 4 {
		t.Fatalf("category/score: %+v", got)
	}
	if got.AISelected == nil || !*got.AISelected || !got.Selected {
		t.Fatalf("selected flags: %+v", got)
	}
```
(score 4 → selected true, ai_selected true, so these still hold.)

Grep the file for any other `Selected:` in an `Enrichment{...}` literal or 0-100 score/relevance and update to 1-5.

- [ ] **Step 2: Run, expect failure**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -run 'ToItem|ProcessBatch' -v`
Expected: FAIL — `Enrichment` has no `Selected` field / selection still floor-based.

- [ ] **Step 3: Rewrite selection in processor.go**

Delete the `selectionRelevanceFloor` const (lines ~12-15) and its comment. In `toItem` (lines ~96-101) replace:
```go
	aiSelected := e.Selected // raw LLM verdict, preserved in ai_selected
	selected := aiSelected || e.Relevance >= selectionRelevanceFloor
```
with:
```go
	// 精选门槛 = B 档及以上 (score>=3). ai_selected now records the stronger
	// "would I feature this" signal (A/S, score>=4) for analytics.
	selected := score >= 3
	aiSelected := score >= 4
```
(`score` is already bound above as `score := e.Score`.)

- [ ] **Step 4: Run, expect pass**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -v`
Expected: PASS (DB-backed tests need `aihot_test`; if unavailable they skip/fail on connection — run at least the non-DB `ToItem` test).

- [ ] **Step 5: Commit**

```bash
git add server/internal/pipeline/processor.go server/internal/pipeline/processor_test.go
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(scoring): 精选 = score>=3, ai_selected = score>=4

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Schema CHECK 1-5 (SQL)

**Files:**
- Modify: `server/internal/items/schema.sql`

- [ ] **Step 1: Tighten the inline CHECKs**

In `server/internal/items/schema.sql`, change lines 19-20:
```sql
    score           INTEGER CHECK (score BETWEEN 1 AND 5),
    ai_relevance    INTEGER CHECK (ai_relevance BETWEEN 1 AND 5),
```
(This governs fresh DBs and the `aihot_test` schema. Existing production `aihot` is migrated with explicit ALTER in Task 8 — the inline change alone does not alter an existing table.)

- [ ] **Step 2: Recreate the test DB schema and run the pipeline suite**

Run:
```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test 2>&1 | tail -30
```
Expected: PASS. (If `make test` recreates `aihot_test` from `schema.sql`, the new CHECK applies and the 1-5 fixtures satisfy it. If a fixture inserts a 0-100 value it will now violate the CHECK — fix that fixture to 1-5.)

- [ ] **Step 3: Commit**

```bash
git add server/internal/items/schema.sql
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(scoring): schema score/ai_relevance CHECK 1-5

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: Frontend scoreLabel / scoreTier (TS)

**Files:**
- Modify: `web/src/format.ts`
- Test: `web/src/format.test.ts`

- [ ] **Step 1: Write the failing tests**

Append to `web/src/format.test.ts`:
```ts
import { scoreLabel, scoreTier } from './format'

describe('scoreLabel', () => {
  it('maps 1-5 to D..S', () => {
    expect(scoreLabel(5)).toBe('S')
    expect(scoreLabel(4)).toBe('A')
    expect(scoreLabel(3)).toBe('B')
    expect(scoreLabel(2)).toBe('C')
    expect(scoreLabel(1)).toBe('D')
  })
  it('clamps out-of-range and handles null', () => {
    expect(scoreLabel(9)).toBe('S')
    expect(scoreLabel(0)).toBe('D')
    expect(scoreLabel(null)).toBe('')
    expect(scoreLabel(undefined)).toBe('')
  })
})

describe('scoreTier', () => {
  it('returns the same letter as scoreLabel for valid input', () => {
    expect(scoreTier(5)).toBe('S')
    expect(scoreTier(1)).toBe('D')
  })
})
```
(Add the `scoreLabel`/`scoreTier` names to the existing top `import` line instead of a second import if your linter forbids duplicate imports — either is fine for vitest.)

- [ ] **Step 2: Run, expect failure**

Run: `cd web && npx vitest run src/format.test.ts`
Expected: FAIL — `scoreLabel`/`scoreTier` not exported.

- [ ] **Step 3: Implement in format.ts**

Append to `web/src/format.ts`:
```ts
const SCORE_LETTERS = ['D', 'C', 'B', 'A', 'S'] as const // index = score-1

// scoreLabel maps a 1-5 tier to its display letter (5→S … 1→D). Out-of-range is
// clamped; null/undefined → '' (no badge).
export function scoreLabel(score: number | null | undefined): string {
  if (score == null) return ''
  const n = Math.max(1, Math.min(5, Math.round(score)))
  return SCORE_LETTERS[n - 1]
}

// scoreTier is the letter used for the `data-tier` styling hook; identical to
// scoreLabel for valid input.
export function scoreTier(score: number | null | undefined): string {
  return scoreLabel(score)
}
```

- [ ] **Step 4: Run, expect pass**

Run: `cd web && npx vitest run src/format.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/format.ts web/src/format.test.ts
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(scoring): scoreLabel/scoreTier 1-5 → S/A/B/C/D

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: ItemCard letter badge + data-tier (TS)

**Files:**
- Modify: `web/src/components/ItemCard.tsx`
- Test: `web/src/components/ItemCard.test.tsx`

- [ ] **Step 1: Update the badge tests**

In `web/src/components/ItemCard.test.tsx`:
- The `base` fixture uses `score: 88` — change to `score: 4` (an A).
- In "renders title, source, category label, score, summary": change `expect(screen.getByText(/88/))` to `expect(screen.getByText('A')).toBeInTheDocument()`.
- In "hides the score badge on non-selected items": change fixture `score: 18` → `score: 1`; keep `expect(document.querySelector('.badge-score')).toBeNull()`; change `expect(screen.queryByText('18')).toBeNull()` → `expect(screen.queryByText('D')).toBeNull()`.
- In "shows the score badge on selected items": change `score: 88` → `score: 5`; change `expect(screen.getByText('88'))` → `expect(screen.getByText('S')).toBeInTheDocument()`; add:
```ts
    expect(document.querySelector('.badge-score')!.getAttribute('data-tier')).toBe('S')
```

- [ ] **Step 2: Run, expect failure**

Run: `cd web && npx vitest run src/components/ItemCard.test.tsx`
Expected: FAIL — badge still renders the number.

- [ ] **Step 3: Update ItemCard.tsx**

Change the import (line 2) to include the helpers:
```tsx
import { categoryLabel, scoreLabel, scoreTier } from '../format'
```
Replace the score badge line (line 17):
```tsx
          {item.selected && item.score != null && (
            <span className="badge-score" data-tier={scoreTier(item.score)}>{scoreLabel(item.score)}</span>
          )}
```

- [ ] **Step 4: Run, expect pass**

Run: `cd web && npx vitest run src/components/ItemCard.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/ItemCard.tsx web/src/components/ItemCard.test.tsx
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(scoring): ItemCard letter badge + data-tier

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 6: Tier badge colors in index.css (CSS)

**Files:**
- Modify: `web/src/index.css`

- [ ] **Step 1: Replace the flat `.badge-score` rule with per-tier colors**

In `web/src/index.css`, the current rule (line ~130) is:
```css
.badge-score { font-size: 12px; color: var(--accent-2); background: var(--accent-soft); border-radius: 999px; padding: 2px 9px; font-weight: 600; font-variant-numeric: tabular-nums; }
```
Replace it with a base rule + tier overrides (light theme):
```css
.badge-score { font-size: 12px; border-radius: 999px; padding: 2px 9px; font-weight: 700; letter-spacing: .3px; }
.badge-score[data-tier="S"] { color: #b7791f; background: #fff4d6; }
.badge-score[data-tier="A"] { color: var(--accent-2); background: var(--accent-soft); }
.badge-score[data-tier="B"] { color: #0f8a4f; background: #e6f7ee; }
.badge-score[data-tier="C"] { color: #6b7078; background: #eef0f2; }
.badge-score[data-tier="D"] { color: #8b9098; background: #f2f3f5; }
```
Then add dark-theme overrides. Find the `:root[data-theme="dark"]` block (starts line ~29) and, after the token list closes (`}` at line ~47), add a sibling rule block:
```css
:root[data-theme="dark"] .badge-score[data-tier="S"] { color: #e5b95b; background: #2a2312; }
:root[data-theme="dark"] .badge-score[data-tier="A"] { color: var(--accent-2); background: var(--accent-soft); }
:root[data-theme="dark"] .badge-score[data-tier="B"] { color: #4fcf8e; background: #10261b; }
:root[data-theme="dark"] .badge-score[data-tier="C"] { color: #9096a0; background: #1c1f25; }
:root[data-theme="dark"] .badge-score[data-tier="D"] { color: #7c828b; background: #191c22; }
```
(Match the file's existing dark-mode convention — if it also uses the `@media (prefers-color-scheme: dark) :root:not([data-theme])` fallback at line ~49, mirror these tier rules there too so system-dark users get them.)

- [ ] **Step 2: Verify the build compiles**

Run: `cd web && npx vitest run && npm run build 2>&1 | tail -5`
Expected: tests PASS, build succeeds (CSS is not unit-tested; this confirms no syntax error).

- [ ] **Step 3: Commit**

```bash
git add web/src/index.css
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(scoring): tier-colored score badges (light+dark)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 7: SSR detail page letter + tier (Go)

**Files:**
- Modify: `server/internal/detailpage/page.go`
- Test: `server/internal/detailpage/page_test.go` (create if absent)

- [ ] **Step 1: Write/extend the page test**

Check for `server/internal/detailpage/page_test.go`. Add a test asserting the badge renders a letter + tier. If the file exists, add this test; if not, create it:
```go
package detailpage

import (
	"strings"
	"testing"

	"aihot-server/internal/items"
)

func TestRenderPageScoreLetterAndTier(t *testing.T) {
	s := 5
	it := items.Item{
		ID: "x", Title: "标题", URL: "https://ex.com/x", Permalink: "/items/x",
		Source: "S", Score: &s, Selected: true,
	}
	html := renderPage(it) // use the actual render entrypoint; see note below
	if !strings.Contains(html, `data-tier="S"`) {
		t.Fatalf("expected data-tier=S in badge: %s", html)
	}
	if !strings.Contains(html, `>S<`) {
		t.Fatalf("expected letter S in badge")
	}
}
```
**Note:** inspect `page.go` for the real render entrypoint (e.g. `renderPage`, `Render`, or the exported handler path) and the `viewModel`/`toViewModel` constructor; call whatever produces the HTML string. If rendering requires a full handler, instead unit-test `toViewModel(it).Score`/`.Tier` directly:
```go
	vm := toViewModel(it)
	if vm.Score != "S" || vm.Tier != "S" { t.Fatalf("vm score/tier: %+v", vm) }
```

- [ ] **Step 2: Run, expect failure**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/detailpage/ -run Score -v`
Expected: FAIL — `Tier` field / letter mapping absent.

- [ ] **Step 3: Add Tier to viewModel and map the letter**

In `server/internal/detailpage/page.go`, add a `Tier string` field to the viewModel struct next to `Score`/`HasScore` (line ~34). In the constructor where `vm.Score = itoa(*it.Score)` (line ~72-74) replace with:
```go
	if it.Score != nil {
		vm.Score = scoreLetter(*it.Score)
		vm.Tier = vm.Score
		vm.HasScore = true
	}
```
Add a helper in the same file:
```go
// scoreLetter maps a 1-5 tier to its display letter (5→S … 1→D), clamped.
func scoreLetter(n int) string {
	letters := []string{"D", "C", "B", "A", "S"}
	if n < 1 {
		n = 1
	}
	if n > 5 {
		n = 5
	}
	return letters[n-1]
}
```
Update the template badge (line ~228):
```go
        {{if .HasScore}}<span class="badge-score" data-tier="{{.Tier}}">{{.Score}}</span>{{end}}
```

- [ ] **Step 4: Add tier colors to the embedded CSS**

In `page.go`'s embedded `<style>`, replace the `.badge-score{...}` rule (line ~176) with the base + per-tier rules (light) and the dark equivalents. The detail page toggles theme via `:root[data-theme="dark"]` (per the theme script), so mirror the same selectors:
```css
.badge-score{font-size:12px;font-weight:700;border-radius:999px;padding:2px 10px;letter-spacing:.3px;}
.badge-score[data-tier="S"]{color:#b7791f;background:#fff4d6;}
.badge-score[data-tier="A"]{color:var(--accent-2);background:var(--accent-soft);}
.badge-score[data-tier="B"]{color:#0f8a4f;background:#e6f7ee;}
.badge-score[data-tier="C"]{color:#6b7078;background:#eef0f2;}
.badge-score[data-tier="D"]{color:#8b9098;background:#f2f3f5;}
:root[data-theme="dark"] .badge-score[data-tier="S"]{color:#e5b95b;background:#2a2312;}
:root[data-theme="dark"] .badge-score[data-tier="B"]{color:#4fcf8e;background:#10261b;}
:root[data-theme="dark"] .badge-score[data-tier="C"]{color:#9096a0;background:#1c1f25;}
:root[data-theme="dark"] .badge-score[data-tier="D"]{color:#7c828b;background:#191c22;}
```
(A already uses theme-var colors so it needs no dark override.)

- [ ] **Step 5: Run, expect pass + build**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/detailpage/ -v && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./...`
Expected: PASS + build OK.

- [ ] **Step 6: Commit**

```bash
git add server/internal/detailpage/page.go server/internal/detailpage/page_test.go
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(scoring): SSR detail page letter badge + tier colors

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 8: Full test, migration, deploy, verify

**Files:** none (build + ops). Requires ssh to Melos and the production DB.

- [ ] **Step 1: Full local test suite**

Run:
```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test 2>&1 | tail -20
cd ../web && npx vitest run 2>&1 | tail -15 && npm run build 2>&1 | tail -5
```
Expected: all PASS, web build succeeds.

- [ ] **Step 2: Cross-compile server + pulse for Melos**

Run (from `server/`; confirm the actual cron/pulse main package paths first with `ls server/cmd`):
```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/aihot-server ./cmd/server
# repeat for the cron/pulse binary that runs the pipeline (e.g. ./cmd/cron or ./cmd/pulse)
```
Expected: binaries produced. (Match whatever binary names `/root/aihot/bin/` and the supervisor config use.)

- [ ] **Step 3: Back up the current score/relevance distribution (safety + before/after)**

Run (read-only), via the project's psql access on Melos:
```sql
SELECT count(*) FILTER (WHERE score IS NOT NULL) AS scored,
       min(score), max(score),
       count(*) FILTER (WHERE selected) AS selected
FROM items;
```
Record the numbers.

- [ ] **Step 4: Run the one-time migration on production `aihot`**

**Order matters — rescale data, then tighten CHECK.** Run as a single transaction:
```sql
BEGIN;

-- Bucket 0-100 → 1-5 for both columns.
UPDATE items SET score = CASE
    WHEN score IS NULL THEN NULL
    WHEN score >= 90 THEN 5
    WHEN score >= 75 THEN 4
    WHEN score >= 60 THEN 3
    WHEN score >= 40 THEN 2
    ELSE 1 END
WHERE score IS NOT NULL AND score > 5;   -- guard: skip rows already migrated

UPDATE items SET ai_relevance = CASE
    WHEN ai_relevance IS NULL THEN NULL
    WHEN ai_relevance >= 90 THEN 5
    WHEN ai_relevance >= 75 THEN 4
    WHEN ai_relevance >= 60 THEN 3
    WHEN ai_relevance >= 40 THEN 2
    ELSE 1 END
WHERE ai_relevance IS NOT NULL AND ai_relevance > 5;

-- Recompute 精选 under the new threshold (score>=3), and ai_selected (score>=4).
UPDATE items SET selected = (score IS NOT NULL AND score >= 3);
UPDATE items SET ai_selected = (score IS NOT NULL AND score >= 4);

-- Tighten the CHECK constraints (auto-named <table>_<column>_check).
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_score_check;
ALTER TABLE items ADD  CONSTRAINT items_score_check CHECK (score BETWEEN 1 AND 5);
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_ai_relevance_check;
ALTER TABLE items ADD  CONSTRAINT items_ai_relevance_check CHECK (ai_relevance BETWEEN 1 AND 5);

COMMIT;
```
**Before COMMIT**, verify the distribution looks sane:
```sql
SELECT score, count(*) FROM items GROUP BY score ORDER BY score;
SELECT count(*) FILTER (WHERE selected) AS now_selected FROM items;
```
The `> 5` guards make the UPDATEs idempotent (safe to re-run). If the constraint name differs, find it: `SELECT conname FROM pg_constraint WHERE conrelid='items'::regclass AND contype='c';`

- [ ] **Step 5: Ship binaries + SPA, restart**

- Copy `/tmp/aihot-server` and the cron binary to `/root/aihot/bin/` on Melos (via scp with `-o ClearAllForwardings=yes`).
- Copy `web/dist/*` to `/var/www/aihot`.
- Restart the server: `ssh -o ClearAllForwardings=yes melos 'supervisorctl restart aihot-server'`.
- (Cron binaries are invoked by `/root/aihot/bin/aihot-cron.sh`; replacing the binary is enough — no restart needed.)

- [ ] **Step 6: Verify end-to-end**

- `curl -s 'http://10.190.12.242:8899/openapi/items?mode=selected&limit=5'` → items return `score` in 1-5.
- Open `http://10.190.12.242:8899/` → 精选 cards show letter badges (S/A/B) with tier colors; 全部 items show no score badge.
- Open a detail page `http://10.190.12.242:8899/items/<id>` → letter badge renders with color; toggle theme → dark colors apply.
- Trigger one pulse cycle (or wait for the :00/:30 cron) and confirm a freshly enriched item lands with a 1-5 score and correct `selected`.

- [ ] **Step 7: Record the live distribution for the follow-up tuning**

Run and save:
```sql
SELECT score, count(*) FROM items WHERE present GROUP BY score ORDER BY score DESC;
```
This is the input for deciding whether the 精选 threshold stays at B or moves (Follow-up in the spec). Report the distribution to the user.

---

## Self-Review

- **Spec coverage:** score 1-5 five tiers (T1), value model in prompt (T1), 精选=score≥3 + drop selected + ai_selected=score≥4 (T2), relevance 1-5 (T1 struct+prompt, T3 schema, T8 migration), schema CHECK 1-5 (T3+T8), migration bucketing both columns + recompute selected (T8), letter display S/A/B/C/D (T4/T5/T7), tier-colored badges light+dark (T6/T7), clamp error handling (T1). Follow-up distribution capture (T8 step 7). All spec sections map to a task.
- **Placeholders:** none — every code step shows the actual code. Two spots ask the implementer to confirm a real name against the codebase (detailpage render entrypoint in T7; cron/pulse main package path + supervisor binary names in T8) because those weren't fully read during planning; each gives a concrete fallback.
- **Type consistency:** `scoreLabel`/`scoreTier` (TS) and `scoreLetter` (Go) each map 1-5→letter with the same `['D','C','B','A','S']` ordering. `data-tier` value = the letter, consistent across ItemCard, index.css, and page.go. `Enrichment.Selected` removed once (T1) and every fixture referencing it updated (T1, T2). CHECK constraint names `items_score_check`/`items_ai_relevance_check` match Postgres auto-naming; T8 provides a lookup query if they differ.
