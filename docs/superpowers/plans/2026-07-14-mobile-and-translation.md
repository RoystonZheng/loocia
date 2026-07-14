# 移动端响应式 + 英文正文翻译 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** (①) 手机窄屏可用的响应式布局；(②) 英文（RSS）源正文入库时用低档模型翻成中文存 `body_cn`，详情页默认出中文并保留切回英文原文的开关。

**Architecture:** 新增 `items.body_cn` 列（照 `reason` 列 lockstep）。新 `pipeline/translate.go` 用独立 llm.Client（低档模型 `deepseek-v4-flash`，env 可配）把 rss 正文翻成中文；接进 `processor.processOne` 做 best-effort（失败不丢条目）。详情页渲染中文 body_cn + 隐藏英文原文 + 内联 JS 切换。一次性 `cmd/backfilltranslate` 回填 228 条。移动端加 `max-width:600px` 断点（React `index.css` + 详情页内嵌 CSS）。

**Tech Stack:** Go (nuwa), PostgreSQL 17 (pgx), React+TS (Vite/vitest). LLM 内网代理 Anthropic Messages API。

**Build/deploy invariants:**
- Go: 每条 `go`/`make` 前缀 `GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org`；`make test` 用 `-p 1`。
- 交叉编译 Melos：加 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`。
- ssh：`ssh -o ClearAllForwardings=yes melos`。
- 提交：`git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit`，尾 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。
- 生产 DB `aihot`（勿 truncate）；测试 DB `aihot_test`（DB 测试无 `AIHOT_TEST_DATABASE_URL` 会自动 skip）。
- LLM key 是密钥（Melos `/root/wechat-push/.env` 的 `LLM_API_KEY`），**永不打印/写文件/提交**。

---

## File Structure

| File | Change |
|---|---|
| `server/internal/items/schema.sql` | 加 `body_cn TEXT` 列 + `ALTER ... ADD COLUMN IF NOT EXISTS` |
| `server/internal/items/item.go` | Item 加 `BodyCN *string` |
| `server/internal/items/write.go` | itemColumns/占位/EXCLUDED/args/scanItem 加 body_cn（→24 列） |
| `server/internal/items/query.go` | listColumns 加 `NULL::text AS body_cn` |
| `server/internal/items/write_test.go` | round-trip 带 body_cn |
| `server/internal/llm/client.go` | 加 `NewTranslateClientFromEnv()`（低档模型） |
| `server/internal/pipeline/translate.go` | 新建：`Translator` 接口 + 实现 + `translateSystemPrompt` |
| `server/internal/pipeline/translate_test.go` | 新建 |
| `server/internal/pipeline/processor.go` | `translator` 字段 + `WithTranslator` + processOne best-effort 翻译 |
| `server/internal/pipeline/processor_test.go` | rss 翻译 / 失败回退 / mp 跳过 |
| `server/internal/pulse/pulse.go` | Deps 加 `Translator` + wire `.WithTranslator` |
| `server/cmd/pulse/main.go` | 组装 translate client + translator |
| `server/internal/detailpage/page.go` | viewModel BodyOriginal/HasTranslation + toViewModel + 模板 toggle + 内嵌 JS |
| `server/internal/detailpage/markdown.go` | rss 有 body_cn 时导出中文 + 注明 |
| `server/internal/detailpage/page_test.go` | 翻译 viewModel + 模板断言 |
| `server/cmd/backfilltranslate/main.go` | 新建：回填 228 条 |
| `web/src/index.css` | `@media (max-width:600px)` 移动端断点 |
| `server/internal/detailpage/page.go` (embedded CSS) | 详情页 600px 断点 |

---

## Task 1: items.body_cn 列 + DB 层 lockstep

**Files:** schema.sql, item.go, write.go, query.go, write_test.go

- [ ] **Step 1: schema.sql**

`server/internal/items/schema.sql`：在 `body TEXT,` 那行后加 `    body_cn         TEXT,`，并在底部 ALTER 区加 `ALTER TABLE items ADD COLUMN IF NOT EXISTS body_cn TEXT;`

- [ ] **Step 2: item.go**

`server/internal/items/item.go` 的 `Item` struct，在 `Body *string` 后加：
```go
	BodyCN         *string
```

- [ ] **Step 3: write_test.go — round-trip 断言（先写测试）**

在 `server/internal/items/write_test.go` 里，找到构造 Item 并 Upsert→GetByID 的测试（`intptr` 已用于 score），给 Item 字面量加一个 `BodyCN`，并在读回后断言。若现有测试用 helper 构造 Item，就近加。示例（若测试有 `it := Item{...}` 字面量）：
```go
	cn := "中文正文"
	// ...在 Item{...} 里加：
	BodyCN: &cn,
	// ...GetByID 读回后加断言：
	if got.BodyCN == nil || *got.BodyCN != "中文正文" {
		t.Fatalf("body_cn round-trip: %v", got.BodyCN)
	}
```
（先读该文件确认现有测试结构，把断言接到已有的 write round-trip 测试里；不要新造整套脚手架。）

- [ ] **Step 4: 跑测试看失败（编译失败：Upsert/scan 列数不匹配）**

`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./internal/items/` → 预期通过（仅加字段还没接线），但 round-trip 断言会因 body_cn 没写库而失败（GetByID 返回 nil）。跑 `go test ./internal/items/ -run Write`（无测试 DB 会 skip；有则 FAIL）。

- [ ] **Step 5: write.go — 24 列 lockstep**

`server/internal/items/write.go`：
1. `itemColumns` 末尾 `reason` 改成 `reason, body_cn`。
2. VALUES 占位加 `,$24`：`...$22,$23,$24)`。
3. ON CONFLICT SET 里 `reason=EXCLUDED.reason,` 后加 `body_cn=EXCLUDED.body_cn,`（放在 `updated_at=now()` 前）。
4. args 末尾 `it.Reason` 改 `it.Reason, it.BodyCN`。
5. `scanItem` 末尾 `&it.Reason` 改 `&it.Reason, &it.BodyCN`。

- [ ] **Step 6: query.go — listColumns 占位**

`server/internal/items/query.go` 的 `listColumns` 末尾 `NULL::text AS reason` 改成 `NULL::text AS reason, NULL::text AS body_cn`（列数与 scanItem 对齐；列表不查真值）。

- [ ] **Step 7: 跑测试**

`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test 2>&1 | tail -15` → 预期全绿（items 包 build + 有测试 DB 时 round-trip 过）。

- [ ] **Step 8: Commit**

```bash
cd /Users/didi/aihot-internal && git add server/internal/items/ && git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(translate): add items.body_cn column (lockstep)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: 翻译单元（llm 低档 client + pipeline/translate.go）

**Files:** llm/client.go, pipeline/translate.go (new), pipeline/translate_test.go (new)

- [ ] **Step 1: llm 低档 client 构造器**

`server/internal/llm/client.go`：在 `NewClientFromEnv` 后加（复用同 key/base，只换 model）：
```go
// defaultTranslateModel is a cheaper tier than auto-max; translation of short
// RSS bodies doesn't need the flagship and shouldn't compete for its RPM.
const defaultTranslateModel = "deepseek-v4-flash"

// NewTranslateClientFromEnv builds a client on the same proxy/key as
// NewClientFromEnv but with the lower-tier translation model (AIHOT_TRANSLATE_MODEL
// override, default deepseek-v4-flash).
func NewTranslateClientFromEnv() (*Client, error) {
	key := os.Getenv("AIHOT_LLM_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("AIHOT_LLM_API_KEY not set")
	}
	base := os.Getenv("AIHOT_LLM_BASE_URL")
	if base == "" {
		base = defaultBaseURL
	}
	model := os.Getenv("AIHOT_TRANSLATE_MODEL")
	if model == "" {
		model = defaultTranslateModel
	}
	return NewClient(base, key, model), nil
}
```

- [ ] **Step 2: translate_test.go（先写测试）**

`server/internal/pipeline/translate_test.go`：
```go
package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTranslatePassesThroughModelOutput(t *testing.T) {
	f := &fakeLLM{reply: "<p>这是中文正文。</p>"}
	tr := NewTranslator(f)
	got, err := tr.Translate(context.Background(), "<p>English body.</p>")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if got != "<p>这是中文正文。</p>" {
		t.Fatalf("translated: %q", got)
	}
	// The English body must reach the model as the user message.
	if !strings.Contains(f.gotUser, "English body") {
		t.Fatalf("body not sent to model: %q", f.gotUser)
	}
}

func TestTranslatePropagatesError(t *testing.T) {
	f := &fakeLLM{err: errors.New("boom")}
	tr := NewTranslator(f)
	if _, err := tr.Translate(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestTranslateSystemPromptKeepsTagsAndTerms(t *testing.T) {
	for _, kw := range []string{"HTML", "标签", "术语"} {
		if !strings.Contains(translateSystemPrompt, kw) {
			t.Fatalf("translate prompt missing %q", kw)
		}
	}
}

func TestTranslateCapsLongBody(t *testing.T) {
	f := &fakeLLM{reply: "ok"}
	tr := NewTranslator(f)
	body := strings.Repeat("word ", 5000) // ~25k chars
	if _, err := tr.Translate(context.Background(), body); err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if n := len([]rune(f.gotUser)); n > maxTranslateRunes+50 {
		t.Fatalf("body not capped: %d runes", n)
	}
}
```
（`fakeLLM` 已存在于 `enricher_test.go`，同包可用。）

- [ ] **Step 3: 跑测试看失败**

`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -run Translate 2>&1 | tail` → FAIL（Translator/translateSystemPrompt 未定义）。

- [ ] **Step 4: translate.go 实现**

`server/internal/pipeline/translate.go`：
```go
package pipeline

import (
	"context"
	"strings"
)

// Translator turns an English (HTML) article body into Chinese, preserving tags.
type Translator interface {
	Translate(ctx context.Context, body string) (string, error)
}

const translateSystemPrompt = `你是专业的科技翻译。把用户给的 HTML 正文翻成简体中文，要求：
1. 保留所有 HTML 标签、图片、链接原样不动，只翻译标签之间的文字内容；
2. 技术术语和产品名（如 Transformer、OpenAI、GPU、embedding）按惯例保留英文，不要硬译；
3. 说人话、通顺自然，不要逐字直译；
4. 只输出翻译后的 HTML，不要任何解释、前后缀或代码块标记。`

// maxTranslateRunes caps the body sent to the model. RSS bodies are short
// (median ~165 chars); the rare long one is truncated to stay under token limits.
const maxTranslateRunes = 8000

type translator struct{ llm LLM }

// NewTranslator wraps an LLM (typically a lower-tier client) as a Translator.
func NewTranslator(llm LLM) Translator { return &translator{llm: llm} }

func (t *translator) Translate(ctx context.Context, body string) (string, error) {
	out, err := t.llm.Complete(ctx, translateSystemPrompt, truncateRunes(body, maxTranslateRunes))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
```
（`LLM` 接口与 `truncateRunes` 已在包内存在。）

- [ ] **Step 5: 跑测试 + build**

`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -run Translate -v 2>&1 | tail -15 && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./...` → PASS + build OK。

- [ ] **Step 6: Commit**

```bash
cd /Users/didi/aihot-internal && git add server/internal/llm/client.go server/internal/pipeline/translate.go server/internal/pipeline/translate_test.go && git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(translate): Translator + low-tier translate client

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: processor 接入 + pulse 装配

**Files:** pipeline/processor.go, pipeline/processor_test.go, pulse/pulse.go, cmd/pulse/main.go

- [ ] **Step 1: processor_test.go（先写测试）**

在 `server/internal/pipeline/processor_test.go` 加一个 fake translator 和测试。fake：
```go
type fakeTranslator struct {
	out string
	err error
}

func (f fakeTranslator) Translate(ctx context.Context, body string) (string, error) {
	return f.out, f.err
}
```
测试（用已有的 `toItem` 路径不够——翻译在 processOne 里，需要 DB。改为直接测 processOne 的行为不现实，故测 `toItem` 不涉及翻译；翻译逻辑放 processOne，用一个不依赖 DB 的小测试覆盖决策）：
```go
func TestShouldTranslate(t *testing.T) {
	// rss with body -> yes; mp -> no; empty body -> no.
	if !shouldTranslate("rss", "hi") {
		t.Fatal("rss with body should translate")
	}
	if shouldTranslate("mp", "hi") {
		t.Fatal("mp should not translate")
	}
	if shouldTranslate("rss", "") {
		t.Fatal("empty body should not translate")
	}
}
```

- [ ] **Step 2: 跑测试看失败**

`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pipeline/ -run ShouldTranslate 2>&1 | tail` → FAIL（shouldTranslate 未定义）。

- [ ] **Step 3: processor.go 接入**

`server/internal/pipeline/processor.go`：
1. Processor struct 加字段：`translator Translator // optional; nil disables translation`。
2. 加方法：
```go
// WithTranslator enables Chinese translation of English (rss) bodies.
func (p *Processor) WithTranslator(t Translator) *Processor {
	p.translator = t
	return p
}

// shouldTranslate reports whether an item's body should be machine-translated:
// English (rss) sources with a non-empty body. MP bodies are already Chinese.
func shouldTranslate(sourceKind, body string) bool {
	return sourceKind == "rss" && strings.TrimSpace(body) != ""
}
```
3. 在 `processOne` 里，`it := toItem(r, e)` 之后、media backfill 附近，加 best-effort 翻译：
```go
	// Translate English (rss) bodies to Chinese, best-effort: a failure just
	// leaves body_cn nil and the detail page falls back to the original.
	if p.translator != nil && it.Body != nil && shouldTranslate(r.SourceKind, *it.Body) {
		if cn, err := p.translator.Translate(ctx, *it.Body); err == nil && strings.TrimSpace(cn) != "" {
			it.BodyCN = &cn
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "translate %s: %v\n", r.ID, err)
		}
	}
```
（确认 `fmt`、`os`、`strings` 已 import；processor.go 目前有 `fmt`、`strings`，需要加 `os`。）

- [ ] **Step 4: pulse.go 装配**

`server/internal/pulse/pulse.go`：
1. `Deps` struct 加：`Translator pipeline.Translator // optional`。
2. `Run` 里 proc 组装处加 `.WithTranslator(d.Translator)`：
```go
	proc := pipeline.NewProcessor(rawStore, itemsStore, pipeline.NewEnricher(d.LLM)).
		WithMediaResolver(ingest.NewOGResolver()).
		WithTranslator(d.Translator)
```
（`WithTranslator(nil)` 安全——nil translator 关闭翻译。）

- [ ] **Step 5: cmd/pulse/main.go 组装**

`server/cmd/pulse/main.go`：在建了 `client`（enricher 用）之后加翻译 client，并传进 Deps：
```go
	translateClient, err := llm.NewTranslateClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "translate llm:", err)
		os.Exit(1)
	}
```
并把 `pulse.Deps{Pool: pool, LLM: client, Sources: sources}` 改成：
```go
	sum, err := pulse.Run(ctx, pulse.Deps{Pool: pool, LLM: client, Translator: pipeline.NewTranslator(translateClient), Sources: sources})
```
（确认 import 了 `aihot-server/internal/pipeline`；若没有则加。）

- [ ] **Step 6: 跑测试 + build**

`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test 2>&1 | tail -15 && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./...` → PASS + build。

- [ ] **Step 7: Commit**

```bash
cd /Users/didi/aihot-internal && git add server/internal/pipeline/processor.go server/internal/pipeline/processor_test.go server/internal/pulse/pulse.go server/cmd/pulse/main.go && git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(translate): best-effort rss body translation in pipeline

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: 详情页中文默认 + 英文切换 + 导出

**Files:** detailpage/page.go, detailpage/markdown.go, detailpage/page_test.go

- [ ] **Step 1: page_test.go（先写测试）**

在 `server/internal/detailpage/page_test.go` 加：
```go
func TestToViewModelTranslation(t *testing.T) {
	en := "<p>English.</p>"
	cn := "<p>中文。</p>"
	rss := &items.Item{ID: "x", Title: "t", URL: "https://e.com/x", Permalink: "/items/x",
		Source: "S", SourceKind: "rss", Body: &en, BodyCN: &cn}
	vm := toViewModel(rss)
	if !vm.HasTranslation {
		t.Fatal("rss+body_cn should have translation")
	}
	if !strings.Contains(string(vm.Body), "中文") {
		t.Fatalf("default body should be Chinese: %q", vm.Body)
	}
	if !strings.Contains(string(vm.BodyOriginal), "English") {
		t.Fatalf("original should be English: %q", vm.BodyOriginal)
	}

	// mp (already Chinese) -> no toggle.
	mpb := "# 标题\n正文"
	mp := &items.Item{ID: "y", Title: "t", URL: "https://e.com/y", Permalink: "/items/y",
		Source: "S", SourceKind: "mp", Body: &mpb}
	vm2 := toViewModel(mp)
	if vm2.HasTranslation {
		t.Fatal("mp should not have translation toggle")
	}

	// rss WITHOUT body_cn (translation failed) -> falls back to English, no toggle.
	rssNoCN := &items.Item{ID: "z", Title: "t", URL: "https://e.com/z", Permalink: "/items/z",
		Source: "S", SourceKind: "rss", Body: &en}
	vm3 := toViewModel(rssNoCN)
	if vm3.HasTranslation {
		t.Fatal("rss without body_cn should not have toggle")
	}
	if !strings.Contains(string(vm3.Body), "English") {
		t.Fatalf("fallback body should be English original: %q", vm3.Body)
	}
}
```
（`strings` 需 import 到测试文件。）

- [ ] **Step 2: 跑测试看失败**

`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/detailpage/ -run Translation 2>&1 | tail` → FAIL。

- [ ] **Step 3: viewModel + toViewModel**

`server/internal/detailpage/page.go`：
1. viewModel struct 在 `Body template.HTML` 后加：
```go
	BodyOriginal   template.HTML // 英文原文, 仅 rss+已翻译时非空
	HasTranslation bool          // true → 展示中文 body + 可切回英文原文
```
2. 替换 toViewModel 里的 body 块（现在是）：
```go
	if it.Body != nil {
		vm.Body = renderBody(it.SourceKind, *it.Body)
	}
```
改成：
```go
	if it.SourceKind == "rss" && it.BodyCN != nil && strings.TrimSpace(*it.BodyCN) != "" && it.Body != nil {
		// English source with a translation: show Chinese by default, keep the
		// English original one toggle away.
		vm.Body = renderBody("rss", *it.BodyCN)
		vm.BodyOriginal = renderBody("rss", *it.Body)
		vm.HasTranslation = true
	} else if it.Body != nil {
		vm.Body = renderBody(it.SourceKind, *it.Body)
	}
```
（`strings` 已在 page.go import。）

- [ ] **Step 4: 模板 — 中文块 + 切换 + 隐藏英文原文**

`server/internal/detailpage/page.go` 模板里，替换这行：
```
      {{if .Body}}<div class="orig-h">原文</div><div class="orig">{{.Body}}</div>{{end}}
```
成：
```
      {{if .Body}}
      <div class="orig-h">原文{{if .HasTranslation}} <button type="button" class="tr-toggle" id="tr-toggle" onclick="__toggleOrig()">看英文原文</button>{{end}}</div>
      <div class="orig" id="orig-cn">{{.Body}}</div>
      {{if .HasTranslation}}<div class="orig" id="orig-en" style="display:none">{{.BodyOriginal}}</div>{{end}}
      {{end}}
```
并在模板底部主题切换 `<script>` 附近（`</body>` 前的 script 块里）加切换函数：
```
<script>
function __toggleOrig(){
  var cn=document.getElementById('orig-cn'), en=document.getElementById('orig-en'), b=document.getElementById('tr-toggle');
  if(!cn||!en||!b) return;
  var showEn = en.style.display==='none';
  en.style.display = showEn?'':'none';
  cn.style.display = showEn?'none':'';
  b.textContent = showEn?'看中文翻译':'看英文原文';
}
</script>
```
（若模板已有一个 `<script>` 主题块，把 `__toggleOrig` 加进同一个 script 即可，别新开重复标签。先读模板确认。）
再在内嵌 CSS 里给按钮一点样式：`.tr-toggle{margin-left:8px;font-size:12px;padding:2px 8px;border:1px solid var(--border-2);border-radius:999px;background:var(--card);color:var(--accent-2);cursor:pointer;}`

- [ ] **Step 5: 模板断言测试**

在 page_test.go 加一个渲染断言（用实际渲染入口——若无导出的 render 函数，测 `pageTemplate.Execute` 到 buffer；先读 page.go 找到 template 变量名，如 `pageTemplate`）：
```go
func TestPageTemplateHasToggle(t *testing.T) {
	en := "<p>English.</p>"; cn := "<p>中文。</p>"
	vm := toViewModel(&items.Item{ID: "x", Title: "t", URL: "https://e.com/x", Permalink: "/items/x",
		Source: "S", SourceKind: "rss", Body: &en, BodyCN: &cn})
	var b strings.Builder
	if err := pageTemplate.Execute(&b, vm); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "看英文原文") || !strings.Contains(out, "__toggleOrig") || !strings.Contains(out, "orig-en") {
		t.Fatalf("toggle markup missing")
	}
}
```
（把模板变量名改成 page.go 里的实际名字。）

- [ ] **Step 6: markdown.go 导出用中文**

`server/internal/detailpage/markdown.go` 的 `## 原文` 块，改成 rss 有 body_cn 时用中文并注明：
```go
	if it.SourceKind == "rss" && it.BodyCN != nil && strings.TrimSpace(*it.BodyCN) != "" {
		b.WriteString("\n## 原文（AI 翻译）\n\n")
		b.WriteString(*it.BodyCN)
	} else if it.Body != nil && strings.TrimSpace(*it.Body) != "" {
		b.WriteString("\n## 原文\n\n")
		b.WriteString(*it.Body)
	}
```
（`strings` 已 import。原来的 `if it.Body != nil...` 块整体替换成上面这段。）

- [ ] **Step 7: 跑测试 + build**

`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/detailpage/ -v 2>&1 | tail -20 && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./...` → PASS + build。

- [ ] **Step 8: Commit**

```bash
cd /Users/didi/aihot-internal && git add server/internal/detailpage/ && git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(translate): detail page Chinese body + English toggle + md export

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: cmd/backfilltranslate 回填工具

**Files:** server/cmd/backfilltranslate/main.go (new)

- [ ] **Step 1: 读 backfillreason 作模板**

读 `server/cmd/backfillreason/main.go` 全文，仿其结构（flag、db pool、query targets、逐条 LLM、UPDATE、计数打印）。

- [ ] **Step 2: 写 main.go**

`server/cmd/backfilltranslate/main.go`：
```go
// backfilltranslate is a one-off: it fills items.body_cn for English (rss)
// items that have a body but no translation yet, using the low-tier translate
// model. Idempotent, safe to re-run; only touches body_cn.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... [AIHOT_TRANSLATE_MODEL=...] ./backfilltranslate [-limit N]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/internal/db"
	"aihot-server/internal/llm"
	"aihot-server/internal/pipeline"
)

func main() {
	limit := flag.Int("limit", 1000, "max rss items to translate")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	client, err := llm.NewTranslateClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm:", err)
		os.Exit(1)
	}
	tr := pipeline.NewTranslator(client)

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT id, body FROM items
		WHERE source_kind='rss' AND body IS NOT NULL AND body <> '' AND body_cn IS NULL
		ORDER BY COALESCE(published_at,'epoch'::timestamptz) DESC
		LIMIT $1`, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	type target struct{ id, body string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.body); err != nil {
			rows.Close()
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		targets = append(targets, t)
	}
	rows.Close()

	var done, failed int
	for _, t := range targets {
		cn, err := tr.Translate(ctx, t.body)
		if err != nil || cn == "" {
			failed++
			fmt.Fprintf(os.Stderr, "translate %s: %v\n", t.id, err)
			continue
		}
		if _, err := pool.Exec(ctx, `UPDATE items SET body_cn=$1, updated_at=now() WHERE id=$2`, cn, t.id); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "update %s: %v\n", t.id, err)
			continue
		}
		done++
	}
	fmt.Printf("backfilltranslate: targets=%d done=%d failed=%d\n", len(targets), done, failed)
}
```

- [ ] **Step 3: build**

`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./cmd/backfilltranslate` → OK。

- [ ] **Step 4: Commit**

```bash
cd /Users/didi/aihot-internal && git add server/cmd/backfilltranslate/ && git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(translate): backfilltranslate one-off tool

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 6: 移动端响应式（CSS）

**Files:** web/src/index.css, server/internal/detailpage/page.go (embedded CSS)

- [ ] **Step 1: React 端 index.css 断点**

`web/src/index.css` 末尾（或已有媒体查询区附近）加：
```css
@media (max-width: 600px) {
  .shell { flex-direction: column; }
  .sidebar {
    width: auto; height: auto; position: sticky; top: 0; z-index: 20;
    flex-direction: row; align-items: center; gap: 6px;
    border-right: none; border-bottom: 1px solid var(--border);
    padding: 8px 12px; overflow-x: auto;
  }
  .sb-logo { margin: 0 8px 0 0; padding: 6px 8px; font-size: 16px; border: none; }
  .sb-nav { flex-direction: row; flex: 1; gap: 2px; }
  .sb-group { display: none; }
  .sb-item { padding: 7px 10px; white-space: nowrap; }
  .sb-theme { border-top: none; padding: 0; margin-left: auto; }
  .main { padding: 16px 14px 40px; }
  .feed-search input { flex: 1; width: auto; min-width: 0; }
  .tl-row { grid-template-columns: 40px 16px 1fr; }
}
```

- [ ] **Step 2: 构建验证**

`cd web && npx vitest run 2>&1 | tail -6 && npm run build 2>&1 | tail -5` → 测试 PASS，build OK（CSS 语法无误）。

- [ ] **Step 3: 详情页内嵌 CSS 断点**

`server/internal/detailpage/page.go` 的内嵌 `<style>` 里找到 shell/sidebar/main 相关规则（详情页也有侧栏壳），在其 CSS 末尾加同款 600px 断点（选择器按详情页实际类名调整；先读该 CSS 段确认类名，通常与 React 端一致 `.shell/.sidebar/.main/.sb-*`）：
```css
@media(max-width:600px){
  .shell{flex-direction:column;}
  .sidebar{width:auto;height:auto;position:sticky;top:0;flex-direction:row;align-items:center;gap:6px;border-right:none;border-bottom:1px solid var(--border);padding:8px 12px;overflow-x:auto;}
  .sb-logo{margin:0 8px 0 0;padding:6px 8px;font-size:16px;border:none;}
  .sb-nav{flex-direction:row;flex:1;gap:2px;}
  .sb-group{display:none;}
  .sb-item{padding:7px 10px;white-space:nowrap;}
  .sb-theme{border-top:none;padding:0;margin-left:auto;}
  .main{padding:16px 14px 40px;}
  .article{max-width:100%;}
}
```
（若详情页侧栏类名不同，用实际类名。目标：窄屏侧栏收顶、正文全宽。）

- [ ] **Step 4: 防回归断言（React 端）**

在 `web/src/format.test.ts` 或新建一个小 css 存在性测试不方便（vitest 不读 css）。改为在 `web/src` 加一个极简断言测试读取 index.css 文本：`web/src/mobile.test.ts`：
```ts
import { readFileSync } from 'node:fs'
import { describe, it, expect } from 'vitest'

describe('mobile breakpoint', () => {
  it('index.css has a 600px breakpoint that restyles the sidebar', () => {
    const css = readFileSync(new URL('./index.css', import.meta.url), 'utf8')
    expect(css).toContain('max-width: 600px')
    const block = css.slice(css.indexOf('max-width: 600px'))
    expect(block).toContain('.sidebar')
  })
})
```

- [ ] **Step 5: 跑测试 + build**

`cd web && npx vitest run 2>&1 | tail -6 && npm run build 2>&1 | tail -5 && cd ../server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./...` → 全绿。

- [ ] **Step 6: Commit**

```bash
cd /Users/didi/aihot-internal && git add web/src/index.css web/src/mobile.test.ts server/internal/detailpage/page.go && git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(mobile): 600px responsive breakpoint (feed + detail page)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 7: 测试全绿 + 部署 + 迁移 + 回填 + 验证

**Files:** none（build + ops）。触碰生产 `aihot`（仅加空列 + 回填 body_cn）、重新部署。

- [ ] **Step 1: 全量本地测试**

```bash
cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test 2>&1 | tail -20
cd ../web && npx vitest run 2>&1 | tail -8 && npm run build 2>&1 | tail -5
```
预期全 PASS。

- [ ] **Step 2: 交叉编译 server + pulse + backfilltranslate**

```bash
cd /Users/didi/aihot-internal/server
GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/aihot-build2/aihot-server .
GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/aihot-build2/pulse ./cmd/pulse
GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/aihot-build2/backfilltranslate ./cmd/backfilltranslate
```
（`mkdir -p /tmp/aihot-build2` 先。server 主包是仓库根的 `main.go`，即 `.`。）

- [ ] **Step 3: 生产加列**

```bash
ssh -o ClearAllForwardings=yes melos 'psql "postgres://aihot:aihot@localhost:5432/aihot" -c "ALTER TABLE items ADD COLUMN IF NOT EXISTS body_cn TEXT;"'
```

- [ ] **Step 4: cron 包装脚本加翻译模型 env**

编辑 Melos 的 `/root/aihot/bin/aihot-cron.sh`，在 `export AIHOT_LLM_API_KEY=...` 附近加一行 `export AIHOT_TRANSLATE_MODEL="deepseek-v4-flash"`。（读现文件再改，勿动 LLM key 那行的取值逻辑。）

- [ ] **Step 5: 发二进制 + SPA + 重启**

- scp 三个二进制到 `/root/aihot/bin/<name>.new`（server 用 aihot-server→server.new），原子 `mv` 覆盖，`chmod +x`。
- rsync `web/dist/` 到 `/var/www/aihot/`。
- `supervisorctl restart aihot-server`；确认 RUNNING + 日志无异常。

- [ ] **Step 6: 回填翻译（228 条）**

```bash
ssh -o ClearAllForwardings=yes melos '/root/aihot/bin/aihot-cron.sh backfilltranslate 2>&1; tail -3 /root/aihot/log/backfilltranslate.log'
```
（aihot-cron.sh 支持 `backfilltranslate` 作为 UNIT 名——它 exec `/root/aihot/bin/$UNIT`。若脚本白名单限制了 UNIT，直接手工 export 三个 env 后跑 `/root/aihot/bin/backfilltranslate`。）预期 `done≈228 failed≈0`。

- [ ] **Step 7: 抽检翻译质量**

```bash
ssh -o ClearAllForwardings=yes melos 'psql "postgres://aihot:aihot@localhost:5432/aihot" -c "SELECT left(title,30), left(body_cn,120) FROM items WHERE source_kind=%'"'"'rss'"'"' AND body_cn IS NOT NULL ORDER BY random() LIMIT 5;"'
```
肉眼扫 5 条中文是否通顺、标签是否保留。质量差就改 `AIHOT_TRANSLATE_MODEL`（如 `glm-5.1`）重跑回填。

- [ ] **Step 8: 端到端验证**

- 手机窄屏（devtools 390px 或真机）开 `http://10.190.12.242:8899/`：侧栏收成顶栏、导航可点、正文全宽。
- 开一条 rss 详情页：默认中文正文 + "看英文原文"按钮，点击能切英文、再点切回；顶栏移动端也收好。
- 开一条 mp 详情页：无翻译按钮、正文照旧。
- `curl 'http://10.190.12.242:8899/items/<rss-id>?format=md'` → `## 原文（AI 翻译）` 出中文。

---

## Self-Review

- **Spec coverage:** body_cn 列(T1)、低档 translate client + Translator(T2)、processor best-effort 接入 + pulse 装配(T3)、详情页中文默认/英文切换/导出注明(T4)、backfilltranslate(T5)、移动端 600px 断点 feed+detail(T6)、部署/加列/env/回填/抽检/验证(T7)。全 spec 段落有对应任务。
- **Placeholders:** 无——每步给了实际代码。三处让实现者先读真实文件确认名字（write_test 现有结构、page.go 模板变量名与 script 块、aihot-cron.sh 内容），各有兜底说明。
- **Type consistency:** `Translator` 接口签名 `Translate(ctx, string)(string,error)` 在 translate.go / fakeTranslator / backfilltranslate / processor 一致。`BodyCN *string` 贯穿 item/write/query/toViewModel/markdown。`NewTranslator(LLM)`、`NewTranslateClientFromEnv()` 名字前后一致。`shouldTranslate("rss",body)` 与 toViewModel 的 `SourceKind=="rss" && BodyCN!=nil` 判定一致。
