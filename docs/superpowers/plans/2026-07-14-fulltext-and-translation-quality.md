# 全文抓取 + 翻译质量/UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** (Phase 1) 抓源站全文替换 rss 的摘要 body；(Phase 2) 在全文上做翻译校验/重译，加详情页返回键、失败重试(auto-std)、声明模型。一次部署，回填链 `backfillarticle → backfilltranslate`。

**Specs:** `docs/superpowers/specs/2026-07-14-full-article-extraction.md`（Phase 1）+ `2026-07-14-translation-quality-and-detail-ux.md`（Phase 2）。任务里未展开的机械细节（lockstep 列等）以 spec 为准。

**Invariants:** Go 命令前缀 `GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org`，在 `server/` 下跑；`make test` 用 `-p 1`；交叉编译加 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`；提交署名 `Claude <noreply@anthropic.com>` 尾 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`；生产 `aihot` 勿 truncate；LLM key 保密。ssh `-o ClearAllForwardings=yes melos`。本机 Mac 可达内网 goproxy（已验证）。

---

## Phase 1 — 全文抓取

### Task 1: go-readability 依赖 + extractArticle

**Files:** `server/go.mod`/`go.sum`, `server/internal/ingest/article.go`(new), `server/internal/ingest/article_test.go`(new)

- [ ] **Step 1: 加依赖**。`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org GOPROXY=https://goproxy.intra.xiaojukeji.com/,direct go get github.com/go-shiori/go-readability@latest`。确认 go.mod 出现该模块。
- [ ] **Step 2: 确认 API**。`GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go doc github.com/go-shiori/go-readability FromReader` 和 `go doc github.com/go-shiori/go-readability Article`。记下 `FromReader` 第二参类型（`*url.URL` 或 `string`）和 `Article.Content` 字段名。
- [ ] **Step 3: 写测试**（`article_test.go`）：
```go
package ingest

import "testing"

func TestExtractArticlePullsMainContent(t *testing.T) {
	html := `<html><head><title>T</title></head><body>
	<nav>menu home about</nav>
	<article><h1>Big News</h1><p>This is the first substantial paragraph of the real article body with enough words to be considered content by the readability algorithm, describing what happened in detail.</p><p>A second paragraph continues the story with more concrete detail and context so the extractor clearly identifies this block as the main content.</p></article>
	<aside>ads sidebar junk</aside></body></html>`
	out, ok := extractArticle([]byte(html), "https://example.com/a")
	if !ok {
		t.Fatal("should extract")
	}
	if !containsAll(out, "substantial paragraph", "second paragraph") {
		t.Fatalf("main content missing: %q", out)
	}
	if containsAny(out, "menu home about", "ads sidebar junk") {
		t.Fatalf("noise not stripped: %q", out)
	}
}

func TestExtractArticleEmptyOnGarbage(t *testing.T) {
	if _, ok := extractArticle([]byte("<html><body></body></html>"), "https://e.com/x"); ok {
		t.Fatal("empty page should not extract")
	}
}

func containsAll(s string, subs ...string) bool { for _, x := range subs { if !stringsContains(s, x) { return false } }; return true }
func containsAny(s string, subs ...string) bool { for _, x := range subs { if stringsContains(s, x) { return true } }; return false }
func stringsContains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int { for i := 0; i+len(sub) <= len(s); i++ { if s[i:i+len(sub)] == sub { return i } }; return -1 }
```
（用 `strings.Contains` 更简单——上面手写只为自足；实现者可直接 `import "strings"` 用 `strings.Contains`，删掉手写 helper。）
- [ ] **Step 4: 跑，看失败**：`GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/ingest/ -run ExtractArticle 2>&1 | tail`（undefined extractArticle）。
- [ ] **Step 5: 实现 `article.go`**（按 Step 2 确认的真实 API 调整）：
```go
package ingest

import (
	"bytes"
	"net/url"
	"strings"

	readability "github.com/go-shiori/go-readability"
)

// extractArticle runs Mozilla-Readability over a page's HTML and returns the
// main-content HTML. ok=false on parse failure or trivially small content.
func extractArticle(htmlBytes []byte, pageURL string) (string, bool) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", false
	}
	art, err := readability.FromReader(bytes.NewReader(htmlBytes), u) // 若该版本第二参是 string，用 pageURL
	if err != nil {
		return "", false
	}
	content := strings.TrimSpace(art.Content)
	if len(strings.TrimSpace(art.TextContent)) < 200 {
		return "", false
	}
	return content, true
}
```
- [ ] **Step 6: 跑通 + build**：`GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/ingest/ -run ExtractArticle -v 2>&1 | tail && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./...`。
- [ ] **Step 7: commit** `server/go.mod server/go.sum server/internal/ingest/article.go server/internal/ingest/article_test.go` — `feat(ingest): readability article extraction`.

### Task 2: OGResolver → PageResolver（抓一次，出媒体+正文）

**Files:** `server/internal/ingest/ogfetch.go`, `server/internal/ingest/ogfetch_test.go`(或 new page_test.go)

- [ ] **Step 1: 测试先行**（用 `net/http/httptest` 起本地 server 返回含 OG + article 的 HTML）：
```go
func TestPageResolverReturnsMediaAndArticle(t *testing.T) {
	page := `<html><head><meta property="og:image" content="https://cdn/x.jpg"></head><body><article><p>` + strings.Repeat("real article words here ", 40) + `</p></article></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){ io.WriteString(w, page) }))
	defer srv.Close()
	r := NewPageResolver()
	img, _, article := r.Resolve(context.Background(), srv.URL)
	if img == nil || *img != "https://cdn/x.jpg" { t.Fatalf("og image: %v", img) }
	if article == nil || !strings.Contains(*article, "real article words") { t.Fatalf("article: %v", article) }
}
```
- [ ] **Step 2: 跑看失败**。
- [ ] **Step 3: 改 `ogfetch.go`**：
  - 把 `FetchOGMedia` 抽成"抓一次拿 htmlBytes"，或新增 `fetchPage(ctx, client, url) ([]byte, bool)`（GET + UA + 非200/err→nil + `io.LimitReader(resp.Body, maxPageBytes)`，`const maxPageBytes = 3<<20`）。
  - 新增 `type PageResolver struct{ client *http.Client }`、`func NewPageResolver() *PageResolver`（client Timeout 15s）。
  - `func (p *PageResolver) Resolve(ctx, pageURL string) (image, video, article *string)`：`b, ok := fetchPage(...)`；ok 则从 `string(b)` 跑 `ogContent(...)` 出 image/video（复用现有逻辑），`extractArticle(b, pageURL)` 出正文；各自非空才设指针。
  - 保留 `OGResolver`/`FetchOGMedia` 或删除？——**删除 OGResolver**（被 PageResolver 取代），`FetchOGMedia` 逻辑并入 fetchPage+ogContent。确保没有其它引用（grep `OGResolver`、`FetchOGMedia`）。
- [ ] **Step 4: 跑通 + build**（`go build ./...` 会因 processor/pulse 仍引用 OGResolver 而失败——这留给 Task 3；本任务确保 ingest 包自身测试过、`go test ./internal/ingest/` 绿；若因跨包引用编译不过，报 DONE_WITH_CONCERNS 说明待 Task 3）。
- [ ] **Step 5: commit** ingest 两文件 — `feat(ingest): PageResolver fetches media + article in one pass`.

### Task 3: processor 用全文 + pulse 装配

**Files:** `pipeline/processor.go`, `pipeline/processor_test.go`, `pulse/pulse.go`, `cmd/pulse/main.go`

- [ ] **Step 1: 测试先行**（`processor_test.go`）：加 fake page resolver + betterBody 测试：
```go
type fakePage struct{ img, vid, article *string }
func (f fakePage) Resolve(ctx context.Context, u string) (image, video, article *string) { return f.img, f.vid, f.article }

func TestBetterBody(t *testing.T) {
	short := "one short line"
	if !betterBody(strings.Repeat("full article text ", 40), &short) { t.Fatal("long extract should win") }
	if betterBody("tiny", &short) { t.Fatal("short extract should not replace") }
	long := strings.Repeat("existing long body ", 50)
	if betterBody("also short", &long) { t.Fatal("must be >=1.5x current") }
}
```
（另加一个 DB 版：rss + fakePage 返回长正文 → GetByID.Body 是全文；正文短 → 保留 snippet；mp → 不替换。仿现有 `TestProcessBatchEnrichesAndMarksProcessed`。）
- [ ] **Step 2: 跑看失败**。
- [ ] **Step 3: 改 processor.go**：
  - 接口 `MediaResolver` → `PageResolver`：`Resolve(ctx, pageURL string) (image, video, article *string)`。字段 `media` → `page`；`WithMediaResolver` → `WithPageResolver`。
  - processOne 的 media 块改成三返回（见 spec §2 代码）：媒体仍仅缺失时补；`if r.SourceKind=="rss" && article != nil && betterBody(*article, it.Body) { it.Body = article }`。
  - 加 `func betterBody(extracted string, cur *string) bool`：`textLen(extracted) >= 300 && (cur==nil || textLen(extracted) >= textLen(*cur)*3/2)`。加 `textLen`（去标签后 rune 数——可复用一个简单 `regexp` 去标签或 `len([]rune(stripTags(s)))`；ingest 无导出 stripTags，processor 里写个小的 `stripTags` 正则 `<[^>]+>`→""）。
- [ ] **Step 4: pulse.go**：`WithMediaResolver(ingest.NewOGResolver())` → `WithPageResolver(ingest.NewPageResolver())`。
- [ ] **Step 5: cmd/pulse**：若直接构造 resolver 则同步改；否则不动。
- [ ] **Step 6: 全测 + build**：`GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test 2>&1 | tail -15 && ... go build ./...`。
- [ ] **Step 7: commit** 四文件 — `feat(pipeline): use extracted full article as rss body`.

### Task 4: cmd/backfillarticle

**Files:** `server/cmd/backfillarticle/main.go`(new)

- [ ] **Step 1: 仿 `cmd/backfillreason/main.go` 结构**写工具：查 `source_kind='rss' AND body IS NOT NULL AND length(regexp_replace(body,'<[^>]+>','','g')) < 500`（短 body）的 (id, url, body)；对每条 `ingest.NewPageResolver().Resolve` 拿 article，`betterBody` 通过则 `UPDATE items SET body=$1, body_cn=NULL, body_cn_model=NULL, updated_at=now() WHERE id=$2`（置空 body_cn 触发重译）。best-effort，抓不到跳过。**串行 + 每条间隔（如 `time.Sleep(300ms)` via context，注意脚本可 sleep）** 对源站礼貌。打印 `targets/updated/skipped`。
  - 注意 `betterBody` 在 pipeline 包是小写不可导出——backfillarticle 里复制一份等价判断，或把长度判断内联（去标签文本长度 ≥300 且 ≥ 原 body 文本 ×1.5）。
  - `body_cn`/`body_cn_model` 列在 Phase 2 Task 5 才加；**本任务依赖 Task 5 已完成**（否则 UPDATE 报列不存在）。→ 执行顺序上 Task 4 放到 Task 5 之后，或本任务 UPDATE 先不写 body_cn（只更 body），置空 body_cn 的动作并入回填链的 SQL。**决定：Task 4 的 UPDATE 只写 body；置空 body_cn 由部署时一条 SQL 统一做**（见 Task 11）。
- [ ] **Step 2: build**：`go build ./cmd/backfillarticle`。
- [ ] **Step 3: commit** — `feat(cmd): backfillarticle full-text fetch tool`.

---

## Phase 2 — 翻译质量 + UX

### Task 5: body_cn_model 列（lockstep）
**Files:** `items/{schema.sql,item.go,write.go,query.go,write_test.go}`
- [ ] 照 `body_cn`/`reason` 列模式加 `body_cn_model TEXT`（25 列）：schema CREATE + ALTER；`Item.BodyCNModel *string`；write.go itemColumns/占位 `$25`/EXCLUDED/args/scanItem；query.go listColumns `NULL::text AS body_cn_model`；write_test round-trip 加断言。跑 `make test` + build。commit `feat(translate): items.body_cn_model column`.

### Task 6: 译文校验 + 改进提示词 + Model()
**Files:** `pipeline/translate.go`, `pipeline/translate_test.go`, `llm/client.go`
- [ ] `llm/client.go` 加 `func (c *Client) Model() string { return c.model }`。
- [ ] `translate.go`：`Translator` 加 `Model() string`；`NewTranslator(llm LLM, model string)` 存 model；impl 返回。`Translate`：`out=TrimSpace(model out)`；`if looksUntranslated(body,out) { return "", errUntranslated }`。加 `var errUntranslated = errors.New("translation looks untranslated/refused")` 和 `looksUntranslated(src,out string) bool`（拒翻关键词组 + 去标签后 CJK 占比 <0.10 且长度>20 + 空；关键词：`请提供 我会按 抱歉 无法翻译 请将 作为一个 as an ai please provide i'll translate`，`strings.ToLower` 比）。改进 `translateSystemPrompt` 增补"必须输出中文/短也翻/别索要内容/别回英文/只输出译文"。
- [ ] 测试：`looksUntranslated` 命中三类、放过正常中文；`Translate` 坏输出→errUntranslated、好输出透传；`Model()` 返回构造名。所有 `NewTranslator(x)` 旧调用改成 `NewTranslator(x, "...")`（grep 全仓：processor_test/backfill/cmd）。
- [ ] make test + build。commit `feat(translate): validate output, keep-CN prompt, Model()`.

### Task 7: processor 存 BodyCNModel
**Files:** `pipeline/processor.go`, `pipeline/processor_test.go`
- [ ] 翻译成功块里 `it.BodyCN = &cn; m := p.translator.Model(); it.BodyCNModel = &m`。测试：成功→两者都设；失败/校验坏→都不设、条目仍存（更新 Task 3 里的 DB 测试或加一个）。make test + build。commit `feat(translate): store per-item translation model`.

### Task 8: 详情页 返回键 + 重试按钮 + 声明模型
**Files:** `detailpage/page.go`, `detailpage/markdown.go`, `detailpage/page_test.go`
- [ ] viewModel 加 `ID string`、`BodyCNModel string`、`CanRetranslate bool`。toViewModel：`vm.ID=it.ID`；HasTranslation 时 `vm.BodyCNModel = deref(it.BodyCNModel, "AI")`；`vm.CanRetranslate = it.SourceKind=="rss" && it.Body!=nil && (it.BodyCN==nil || strings.TrimSpace(*it.BodyCN)=="")`。
- [ ] 模板：
  - topbar 加返回键 `<a class="back" href="/">← 返回</a>`（放最前）。
  - 原文区：`HasTranslation` 时在切换按钮旁/译文下加 `<p class="tr-note">本文由 AI（{{.BodyCNModel}}）翻译</p>`。
  - `CanRetranslate && !HasTranslation` 时加 `<button id="tr-retry" onclick="__retranslate('{{.ID}}')">翻译失败 · 点此重试翻译</button>`。
  - JS 加 `window.__retranslate=function(id){var b=document.getElementById('tr-retry');if(b){b.disabled=true;b.textContent='翻译中…';}fetch('/items/'+id+'/retranslate',{method:'POST'}).then(function(r){return r.json();}).then(function(j){if(j&&j.ok){location.reload();}else{if(b){b.disabled=false;b.textContent='重试仍失败，可稍后再试';}}}).catch(function(){if(b){b.disabled=false;b.textContent='重试失败，可稍后再试';}});};`（放进现有 IIFE，暴露 window.）
  - CSS：`.back{...小链接}` `.tr-note{font-size:12px;color:var(--muted);margin-top:8px;}` `#tr-retry{...按钮}`。
- [ ] markdown.go：`## 原文（AI 翻译）` → `## 原文（AI 翻译 · <model>）`（rss+body_cn 时带 `*it.BodyCNModel`，nil 则 "AI"）。
- [ ] 测试：HasTranslation→有 tr-note+模型名+返回键；rss 无 body_cn→有 tr-retry+无 tr-note；mp→都无但有返回键；模板 Execute 断言各标记。make test + build。commit `feat(detail): back link, retry button, model note`.

### Task 9: 重试端点 + 装配
**Files:** `detailpage/handler.go`(或新 handler), `items/write.go`(加 UpdateBodyCN), `main.go`(server 装配)
- [ ] `items` 加 `func (s *Store) UpdateBodyCN(ctx, id, cn, model string) error`（`UPDATE items SET body_cn=$2, body_cn_model=$3, updated_at=now() WHERE id=$1`）。
- [ ] handler：`POST /items/{id}/retranslate`。load item（GetByID）；`if it.SourceKind!="rss" || it.Body==nil { 400 }`；用注入的 **retry translator（auto-std）** `Translate(ctx, *it.Body)`；成功→`UpdateBodyCN(id, cn, model)` + `200 {"ok":true}`；err/空→`200 {"ok":false}`。路由挂载（看 main.go/detailpage 现有 mux 注册方式，仿之）。
- [ ] server main：构造 `retryTr := pipeline.NewTranslator(retryClient, retryClient.Model())`，`retryClient` = `llm.NewTranslateClientFromEnv` 但用 `AIHOT_TRANSLATE_RETRY_MODEL`（默认 auto-std）——加 `llm.NewRetryTranslateClientFromEnv()`（读 `AIHOT_TRANSLATE_RETRY_MODEL` 默认 `auto-std`，同 key/base）。注入 detailpage handler + items store。
- [ ] 测试：handler test——rss+body+fake好译文→ok:true 且 store 被更新；非 rss→400；坏译文→ok:false。make test + build。commit `feat(detail): POST retranslate endpoint (auto-std)`.

### Task 10: backfilltranslate 存模型
**Files:** `cmd/backfilltranslate/main.go`
- [ ] 构造 `tr := pipeline.NewTranslator(client, client.Model())`；成功 UPDATE 改为 `SET body_cn=$1, body_cn_model=$2`（带 `tr.Model()`）。build。commit `feat(cmd): backfilltranslate records model`.

---

## Task 11: 全测 + 部署 + 回填链 + 验证（触生产）

**Files:** none（ops）。**执行前与用户确认。**

- [ ] **全测**：`make test`（Go 全绿）+ web `npx vitest run && npm run build`。
- [ ] **交叉编译**（本机 Mac，内网 goproxy 可达）：`aihot-server`(`.`)、`pulse`、`backfillarticle`、`backfilltranslate` → linux/amd64 到 `/tmp/aihot-build4/`。
- [ ] **生产加列**：`ssh ... 'psql "$AIHOT" -c "ALTER TABLE items ADD COLUMN IF NOT EXISTS body_cn_model TEXT;"'`。
- [ ] **server 启动包装**（给 server 注入 LLM key + 重试模型）：写 `/root/aihot/bin/aihot-server.sh`：
```bash
#!/usr/bin/env bash
set -euo pipefail
export AIHOT_DATABASE_URL="postgres://aihot:aihot@localhost:5432/aihot"
export AIHOT_LLM_API_KEY="$(grep '^LLM_API_KEY' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
export AIHOT_TRANSLATE_RETRY_MODEL="auto-std"
exec /root/aihot/bin/server -c /root/aihot/conf/app.toml "$@"
```
`chmod +x`；改 `/etc/supervisor/conf.d/aihot-server.conf` 的 `command=/root/aihot/bin/aihot-server.sh`；`supervisorctl reread && supervisorctl update`。
- [ ] **发布**：原子替换 `server/pulse/backfillarticle/backfilltranslate`（`.new`→`chmod`→`mv -f`）；rsync `web/dist/` → `/var/www/aihot/`；`supervisorctl restart aihot-server`；确认 RUNNING + 日志正常 + 重试端点能起（server 有 key）。
- [ ] **回填链**：
  1. `aihot-cron.sh backfillarticle`（或直接跑二进制，需 DATABASE_URL）——抓全文更新 body。
  2. 一条 SQL：把仍坏/短的 body_cn 置空重译触发（`UPDATE items SET body_cn=NULL, body_cn_model=NULL WHERE source_kind='rss' AND body_cn IS NOT NULL AND (<Phase2 §7 拒翻/回英文判据>)`）。
  3. `aihot-cron.sh backfilltranslate`——全文上重译（带校验，存模型）。
- [ ] **验证**：
  - 一条原本一句话的 rss 详情页 → 现出完整正文。
  - 坏译文页 → 出「翻译失败·点此重试」→ 点击 → 出中文 + 「本文由 AI（auto-std）翻译」。
  - 返回键跳 feed；mp 页无重试无声明。
  - 抽检若干译文质量（全文 + 校验后应显著改善）；报告最终 好/坏 计数。

---

## Self-Review
- **Spec coverage:** Phase1 extractArticle(T1)/PageResolver(T2)/processor全文(T3)/backfillarticle(T4)；Phase2 body_cn_model(T5)/校验+提示词+Model(T6)/存模型(T7)/详情页返回+重试+声明(T8)/重试端点(T9)/backfill存模型(T10)；部署回填链(T11)。全覆盖。
- **依赖顺序:** T4 依赖 T5 的列——已在 T4 note 里处理（T4 只更 body，置空 body_cn 由 T11 SQL 做）；执行按 T1→T11 顺序即安全。
- **Placeholders:** 无（机械 lockstep 引 spec 的 body_cn 模式，已在前一轮实现过同款）。
- **类型一致:** `PageResolver.Resolve→(image,video,article *string)` 跨 ingest/processor/pulse/test 一致；`NewTranslator(llm,model)` + `Model()` + `Client.Model()` 一致；`body_cn_model`/`BodyCNModel` 贯穿；`errUntranslated`/`looksUntranslated` 在 translate 内。
