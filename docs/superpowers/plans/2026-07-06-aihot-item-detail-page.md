# Item Detail Page (`/items/{id}` SSR) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every permalink (`/items/{id}`, used by feed cards, daily items, and hot-topics) resolve to a real, shareable, server-rendered detail page — Chinese title + full summary + metadata + a prominent 阅读原文 link out to the source — served by the Go server as HTML, deliberately NOT as a JSON API.

**Architecture:** The openapi contract (lines 450-457) is explicit: the permalink page is "中文翻译 + 富文本 + 无墙", it is `noindex`, and "站内正文本身不经 API 直出(去 permalink 页读),避免批量扒全文" — the anti-scrape stance this whole project embodies. So the detail page is Go **server-side rendered HTML** at `GET /items/{id}` (NOT under `/api/`, and NO single-item JSON endpoint). A new `internal/detailpage` package renders one `items.Item` via `html/template` (auto-escaping guards against injection), gated to visible items (present && not a duplicate). The React SPA needs ZERO code changes — permalinks are already `<a href="/items/{id}">` full navigations; we only add `/items` to the Vite dev proxy so dev mirrors production (a reverse proxy routes `/items/*` and `/api/*` to Go, everything else to the static SPA). The page shows our derived Chinese content (title + summary) + metadata + the original English title + an 阅读原文 link to the third-party `url`; it does NOT republish source body text.

**Tech Stack:** Go (`internal/detailpage`, `html/template`), the existing `items.Store`; a one-line Vite proxy addition.

---

## Environment / how to run

- Go via `GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org` (or `make build`/`make test` `-p 1`). Single package: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/detailpage/ -run <name> -v` (httptest — NO DB needed for Task 1; a live-DB check in Task 2).
- Live PG tunnel on localhost:5432 (restart `ssh -N -L 5432:localhost:5432 melos &`). Prod DB `aihot`, test DB `aihot_test`.
- Front-end: `cd web && npm run dev` (:5173, proxies `/api`→:8991; Task 2 adds `/items`).

## Contract recap (openapi Item + permalink, lines 438-468)

- Public `Item`: id, title (中文为主), title_en (仅当与 title 不同), url (第三方原文), permalink (`/items/{id}`), source (always a string), publishedAt (nullable), summary (中文, nullable), category, score, selected.
- The permalink page: 中文翻译 + 富文本 + 无墙; **`noindex`**; body NOT served via API; `url` remains the third-party original.

## File Structure

- `server/internal/detailpage/handler.go` — `ItemGetter` interface, `Handler`, `NewHandler`, `ServeHTTP` (path→id→GetByID→visibility gate→render/404).
- `server/internal/detailpage/page.go` — `html/template` + `viewModel` + Beijing-time / category-label helpers (self-contained; the Go side has no equivalent of the web `format.ts`).
- `server/internal/detailpage/*_test.go` — httptest rendering + 404 + escaping tests.
- `server/common/server/httpserv/httpserv.go` — mount `/items/` subtree.
- `web/vite.config.ts` — add `/items` to the dev proxy.

---

### Task 1: SSR detail page (render + visibility gate)

**Files:**
- Create: `server/internal/detailpage/page.go`
- Create: `server/internal/detailpage/handler.go`
- Test: `server/internal/detailpage/handler_test.go`

- [ ] **Step 1: Write the failing test**

`server/internal/detailpage/handler_test.go`:
```go
package detailpage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aihot-server/internal/items"
)

// fakeGetter serves canned items by id.
type fakeGetter struct{ byID map[string]*items.Item }

func (f fakeGetter) GetByID(ctx context.Context, id string) (*items.Item, error) {
	if it, ok := f.byID[id]; ok {
		return it, nil
	}
	return nil, items.ErrNotFound
}

func mkItem(id string) *items.Item {
	pub := time.Date(2026, 5, 7, 4, 0, 0, 0, time.UTC) // 12:00 Beijing
	en := "Original English Title"
	summary := "这是中文摘要。"
	cat := items.CategoryAIModels
	score := 88
	return &items.Item{
		ID: id, Title: "中文标题", TitleEN: &en, URL: "https://source.example/post",
		Permalink: "/items/" + id, Source: "OpenAI Blog", PublishedAt: &pub,
		Summary: &summary, Category: &cat, Score: &score, Selected: true, Present: true,
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr
}

func TestRendersItemPage(t *testing.T) {
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"abc": mkItem("abc")}})
	rr := get(t, h, "/items/abc")
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type: %q", ct)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"<!doctype html>", `<meta name="robots" content="noindex"`,
		"中文标题", "这是中文摘要。", "OpenAI Blog", "模型发布/更新",
		"2026-05-07 12:00", "Original English Title",
		`href="https://source.example/post"`, "阅读原文", // outbound
		`href="/"`, "返回", // back to feed
		"<title>中文标题", // page title
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in page:\n%s", want, body)
		}
	}
	// The third-party original body must NOT be dumped (anti-scrape) — we only
	// have summary; assert no raw-body field leaks (there is none to leak, but
	// guard the intent): the source url appears only as an href, not inlined text.
}

func TestMissingItemIs404(t *testing.T) {
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{}})
	rr := get(t, h, "/items/nope")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "未找到") {
		t.Fatalf("404 page should be friendly HTML: %s", rr.Body.String())
	}
}

func TestHiddenItemsAre404(t *testing.T) {
	withdrawn := mkItem("w")
	withdrawn.Present = false
	dupID := "canonical"
	duplicate := mkItem("d")
	duplicate.DuplicateOfID = &dupID
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"w": withdrawn, "d": duplicate}})
	if rr := get(t, h, "/items/w"); rr.Code != http.StatusNotFound {
		t.Fatalf("withdrawn should 404, got %d", rr.Code)
	}
	if rr := get(t, h, "/items/d"); rr.Code != http.StatusNotFound {
		t.Fatalf("duplicate should 404, got %d", rr.Code)
	}
}

func TestEmptyIDIs404(t *testing.T) {
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{}})
	if rr := get(t, h, "/items/"); rr.Code != http.StatusNotFound {
		t.Fatalf("empty id should 404, got %d", rr.Code)
	}
}

func TestHTMLIsEscaped(t *testing.T) {
	evil := mkItem("x")
	evil.Title = `<script>alert(1)</script>`
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"x": evil}})
	body := get(t, h, "/items/x").Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatalf("title must be escaped, not raw: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("expected escaped title: %s", body)
	}
}

func TestNullableFieldsOmitted(t *testing.T) {
	minimal := &items.Item{
		ID: "m", Title: "仅标题", URL: "https://s/x", Permalink: "/items/m",
		Source: "S", Present: true,
	}
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"m": minimal}})
	rr := get(t, h, "/items/m")
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "&lt;nil&gt;") || strings.Contains(body, "<nil>") {
		t.Fatalf("nil pointer leaked into page: %s", body)
	}
	if !strings.Contains(body, "仅标题") {
		t.Fatalf("title should render: %s", body)
	}
}
```

- [ ] **Step 2: Run to confirm FAIL** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/detailpage/ -v` → FAIL (`undefined: NewHandler`).

- [ ] **Step 3: Implement**

`server/internal/detailpage/page.go`:
```go
package detailpage

import (
	"html/template"
	"time"

	"aihot-server/internal/items"
)

var categoryLabels = map[string]string{
	items.CategoryAIModels:   "模型发布/更新",
	items.CategoryAIProducts: "产品发布/更新",
	items.CategoryIndustry:   "行业动态",
	items.CategoryPaper:      "论文研究",
	items.CategoryTip:        "技巧与观点",
}

// viewModel is the flattened, render-ready shape (no nil pointers reach the template).
type viewModel struct {
	Title      string
	TitleEN    string // "" when absent
	Summary    string // "" when absent
	Source     string
	URL        string
	Category   string // localized label, "" when absent
	PublishedAt string // "YYYY-MM-DD HH:MM" Beijing, "" when absent
	Score      string // "" when absent
	HasScore   bool
}

func toViewModel(it *items.Item) viewModel {
	vm := viewModel{Title: it.Title, Source: it.Source, URL: it.URL}
	if it.TitleEN != nil {
		vm.TitleEN = *it.TitleEN
	}
	if it.Summary != nil {
		vm.Summary = *it.Summary
	}
	if it.Category != nil {
		if lbl, ok := categoryLabels[*it.Category]; ok {
			vm.Category = lbl
		} else {
			vm.Category = *it.Category
		}
	}
	if it.PublishedAt != nil {
		vm.PublishedAt = it.PublishedAt.In(beijing).Format("2006-01-02 15:04")
	}
	if it.Score != nil {
		vm.Score = itoa(*it.Score)
		vm.HasScore = true
	}
	return vm
}

// beijing is UTC+8 fixed (no DST) — matches the front-end formatBeijingTime.
var beijing = time.FixedZone("CST", 8*3600)

func itoa(i int) string {
	// small helper to avoid importing strconv just for one call site
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

// pageTemplate is self-contained (inline CSS, no external assets), noindex,
// dark-mode aware, green accent matching the SPA.
var pageTemplate = template.Must(template.New("item").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Title}} · AI HOT（内网）</title>
<style>
:root { color-scheme: light dark; }
body { max-width: 720px; margin: 0 auto; padding: 32px 18px; font: 16px/1.6 system-ui, sans-serif; color: #1a1a1a; background: #fff; }
a { color: #1a7f5a; }
.back { display: inline-block; margin-bottom: 20px; text-decoration: none; font-size: .9rem; }
h1 { font-size: 1.6rem; line-height: 1.3; margin: 0 0 6px; }
.title-en { color: #888; font-size: 1rem; font-weight: 400; margin: 0 0 14px; }
.meta { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; font-size: .85rem; color: #777; margin-bottom: 20px; }
.cat { background: #eef6f2; color: #1a7f5a; padding: 1px 8px; border-radius: 999px; }
.score { color: #1a7f5a; font-weight: 600; }
.summary { font-size: 1.05rem; margin: 0 0 28px; }
.readmore { display: inline-block; padding: 10px 18px; background: #1a7f5a; color: #fff; border-radius: 8px; text-decoration: none; }
.note { margin-top: 24px; font-size: .8rem; color: #aaa; }
@media (prefers-color-scheme: dark) {
  body { background: #16171d; color: #e6e6ea; }
  .title-en, .meta, .note { color: #9aa; }
  .cat { background: #14342a; }
}
</style>
</head>
<body>
<a class="back" href="/">← 返回 AI HOT</a>
<h1>{{.Title}}</h1>
{{if .TitleEN}}<p class="title-en">{{.TitleEN}}</p>{{end}}
<div class="meta">
  <span>{{.Source}}</span>
  {{if .Category}}<span class="cat">{{.Category}}</span>{{end}}
  {{if .PublishedAt}}<span>{{.PublishedAt}}</span>{{end}}
  {{if .HasScore}}<span class="score">{{.Score}}</span>{{end}}
</div>
{{if .Summary}}<p class="summary">{{.Summary}}</p>{{end}}
<a class="readmore" href="{{.URL}}" target="_blank" rel="noopener nofollow">阅读原文 →</a>
<p class="note">本页为站内中文精选呈现；正文版权归原信源所有，点击「阅读原文」查看完整内容。</p>
</body>
</html>`))

var notFoundTemplate = template.Must(template.New("404").Parse(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="robots" content="noindex"><title>未找到 · AI HOT（内网）</title>
<style>body{max-width:600px;margin:0 auto;padding:48px 18px;font:16px/1.6 system-ui,sans-serif;text-align:center;color-scheme:light dark;}a{color:#1a7f5a;}</style>
</head>
<body>
<h1>未找到该资讯</h1>
<p>这条内容可能已下线或不存在。</p>
<a href="/">← 返回 AI HOT</a>
</body>
</html>`))
```

`server/internal/detailpage/handler.go`:
```go
package detailpage

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"aihot-server/internal/items"
)

// ItemGetter is the store surface the page needs (satisfied by *items.Store).
type ItemGetter interface {
	GetByID(ctx context.Context, id string) (*items.Item, error)
}

// Handler server-side-renders GET /items/{id} as an HTML page.
type Handler struct {
	store ItemGetter
}

func NewHandler(store ItemGetter) *Handler {
	return &Handler{store: store}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/items/")
	if id == "" || strings.Contains(id, "/") {
		h.renderNotFound(w)
		return
	}

	it, err := h.store.GetByID(r.Context(), id)
	if errors.Is(err, items.ErrNotFound) {
		h.renderNotFound(w)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Visibility gate: withdrawn (not present) or a "完全重复" duplicate → 404.
	if !it.Present || it.DuplicateOfID != nil {
		h.renderNotFound(w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	if err := pageTemplate.Execute(w, toViewModel(it)); err != nil {
		// header already sent on partial write; best-effort log-free fallback
		return
	}
}

func (h *Handler) renderNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(http.StatusNotFound)
	_ = notFoundTemplate.Execute(w, nil)
}

// compile-time assertion that *items.Store satisfies ItemGetter
var _ ItemGetter = (*items.Store)(nil)
```

- [ ] **Step 4: Run to confirm PASS** — `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/detailpage/ -v` → all PASS. (No DB — pure httptest.)

- [ ] **Step 5: Commit**
```bash
git add server/internal/detailpage/
git commit -m "feat(detailpage): SSR /items/{id} page"
```

---

### Task 2: Mount + Vite proxy + live verify

**Files:**
- Modify: `server/common/server/httpserv/httpserv.go`
- Modify: `web/vite.config.ts`

- [ ] **Step 1: Mount the route.** READ `httpserv.go` first; next to the existing mounts (which share `pool` and build `items.New(pool)` — the items store is already constructed there for the items handler; reuse that same `itemsStore` variable), add:
```go
svr.AddHTTPHandle("/items/", detailpage.NewHandler(itemsStore))
```
(plus the `"aihot-server/internal/detailpage"` import.) `http.ServeMux` routes the `/items/` subtree here; `/api/...` and `/healthz` are unaffected. If the existing code named the items store differently, use that name; if the items store is only built inside an `if pool != nil` block, mount the detail route in the same place so it shares the store (with a nil pool the route can still register — GetByID will 500/404 at request time, acceptable).

- [ ] **Step 2: Add the Vite dev proxy.** In `web/vite.config.ts`, the `server.proxy` currently has `'/api': 'http://localhost:8991'`. Add a sibling so `/items/*` full-navigations in dev hit the Go SSR page (mirroring production, where a reverse proxy sends `/items/*` and `/api/*` to Go and everything else to the static SPA):
```ts
    proxy: {
      '/api': 'http://localhost:8991',
      '/items': 'http://localhost:8991',
    },
```
Do NOT break the existing `/api` entry. (This does not affect the hermetic vitest suite — it only touches the dev server.)

- [ ] **Step 3: Front-end suite still green** — `cd web && npx vitest run` → all pass (no test depends on `/items` routing; the change is dev-server-only). `npm run build` → green.

- [ ] **Step 4: Backend build + full suite** — `cd server && make build` → green; `AIHOT_TEST_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot_test' make test` → all packages pass (detailpage + no regressions).

- [ ] **Step 5: Live verify (needs tunnel + prod DB).** Boot against prod: `cd server && AIHOT_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot' make run &` (→ :8991). Pick a real id: `ssh melos "psql 'postgres://aihot:aihot@localhost:5432/aihot' -tAc \"SELECT id FROM items WHERE present AND duplicate_of_id IS NULL ORDER BY published_at DESC LIMIT 1\""`. Then:
   - `curl -si "localhost:8991/items/<that-id>" | head -8` → `200`, `Content-Type: text/html`, `X-Robots-Tag: noindex`; body has the Chinese title + 阅读原文.
   - `curl -si "localhost:8991/items/does-not-exist" | head -1` → `404`.
   - Confirm the API routes still work: `curl -s localhost:8991/api/public/items?take=1 | head -c 80` → JSON.
   Then start the web dev server (`cd web && npm run dev &`) and use Playwright: navigate `http://localhost:5173`, click the first feed item's title link, verify the URL becomes `/items/<id>` and the SSR detail page renders (Chinese title, summary, source, 阅读原文 button, ← 返回 link); click 返回 → back at the feed. Screenshot to `/Users/didi/Downloads/item-detail-page.png`. Check console clean. Kill both servers.

- [ ] **Step 6: Commit**
```bash
git add server/common/server/httpserv/httpserv.go web/vite.config.ts
git commit -m "feat(detailpage): mount /items/ + vite dev proxy"
```

---

## Self-Review

- **Spec coverage:** Delivers the openapi permalink page (lines 450-457): server-rendered `/items/{id}` HTML, `noindex` (meta + `X-Robots-Tag`), Chinese title + summary + metadata + original title + 阅读原文 link to the third-party `url`, ← 返回 to the feed; NO single-item JSON endpoint and NO source-body republishing (honors "站内正文不经 API 直出,避免批量扒全文"). Visibility gate matches item semantics (withdrawn/duplicate → 404; cluster secondaries — which have `duplicate_of_id = NULL` — remain viewable as real distinct items). Front-end permalinks resolve with zero React changes; the Vite proxy makes dev mirror production. Deliberately deferred (with a home): full Chinese-translated 富文本正文 (our pipeline produces a summary, not a translated body — showing the summary + outbound link is the honest scope), production reverse-proxy config for `/items/*` (belongs with the "serve from Melos" follow-on), and og/social meta tags (nice-to-have).
- **Placeholder scan:** No TBD/TODO. Every step has complete code or an exact command. Task 2's mount step references the existing `httpserv.go` items-store variable (implementer reads the file) rather than fabricating its surrounding code — consistent with prior plans.
- **Type consistency:** `ItemGetter.GetByID(ctx, id) (*items.Item, error)` matches `*items.Store.GetByID` (compile-time asserted via `var _ ItemGetter = (*items.Store)(nil)`); `items.ErrNotFound` drives 404; `toViewModel(*items.Item) viewModel` flattens all nullable pointers to safe strings (no `<nil>` reaches the template — pinned by `TestNullableFieldsOmitted`); `html/template` auto-escaping pinned by `TestHTMLIsEscaped`; `categoryLabels` uses the same `items.Category*` constants and Chinese labels as the front-end `format.ts` and the daily builder. Beijing time via a fixed +8 zone matches `formatBeijingTime`.

## Follow-on

- **Serve `/items/*` from Melos** behind the reverse proxy when the stack goes 24/7 (paired with the "serve API+web from Melos" follow-on).
- **Full 富文本正文** if/when the pipeline gains body translation (currently summary-only, by design).
- **Social/OG meta** for nicer link unfurls in D-Chat shares.
