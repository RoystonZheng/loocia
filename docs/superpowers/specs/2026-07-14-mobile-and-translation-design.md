# 移动端响应式 + 英文正文翻译 — Design

**Date:** 2026-07-14
**Status:** Approved (co-designed with user)
**Goal:** (①) 让 feed/详情页在手机上可用；(②) 英文（RSS）源的正文不再直显生英文，入库时翻成中文，详情页默认出中文并保留切回原文的开关。

两块相互独立，一起实现、一起发。③ 知识图谱另立项目，不在本 spec。

---

## ① 移动端响应式修复

**现状**：`.shell` 是 flex 行布局 + 写死 180px `sticky` 全高侧栏，任何屏宽都常驻不折叠；只有 900px 的几个小断点，无手机断点。iPhone(375px) 上侧栏吃掉 180px，正文只剩 ~195px。

**方案**：加 `@media (max-width: 600px)` 断点，纯 CSS 为主（尽量不改 JSX）：
- `.shell` → `flex-direction: column`，侧栏堆到顶部。
- `.sidebar` → 变成顶部 app-bar：`width:auto; height:auto; position: sticky; top:0`，`flex-direction: row; align-items: center`，`border-right` 换 `border-bottom`，padding 收窄。
- `.sb-logo` 缩小、去掉多余 padding；`.sb-nav` 横向排（`flex-direction: row`）；`.sb-group` 分组小标题在窄屏 `display:none`；`.sb-theme` 靠右内联（去掉 `border-top`，改成左边框或无）。
- `.main` 全宽、padding 再收（已有 900px 的 `20px 16px`，600px 再紧一点）。
- 时间线 `.tl-row` 的 `52px 24px 1fr` 在窄屏缩成更小的日期/轨道列（如 `40px 16px 1fr`），避免正文被挤。
- 卡片/feed-controls 已 `flex-wrap`，无需大改；`.feed-search input` 的固定 `width:200px` 在窄屏改成 `flex:1`/`width:auto` 防溢出。

**导航形态**：顶部 app-bar 横向内联（logo 居左 + 精选/全部/日报 + 主题切换靠右）。只有 3 个目的地，不做汉堡抽屉，最少 JS。

**详情页 SSR**：详情页有自己的一套内嵌 CSS + 侧栏壳，也要加同样的 600px 断点（侧栏收顶栏、正文全宽）。

**验证**：CSS 不做单测。用构建 + 人工窄屏核对（浏览器 devtools 375/390px，或线上手机）。可加一个轻量断言：`index.css` 含 `max-width: 600px` 断点、且该断点内出现 `.sidebar` 规则（防回归被删）。

---

## ② 英文正文翻译（`body_cn`）

**目标源**：`source_kind='rss'`（228 条，全英文源：TechCrunch/OpenAI/Google AI/MIT/Simon Willison…）。`mp`（公众号）本就中文，跳过。RSS 正文短（均 820 字、中位 165 字、仅 3 条 >8k），整篇翻不贵。

假设：`rss ≈ 英文`。用 source_kind 判定，不做语言检测（YAGNI；未来若有中文 RSS 顶多多翻一次，模型近似原样返回，可接受）。

### 数据层
- `items` 加列 `body_cn TEXT`（+ `ALTER TABLE items ADD COLUMN IF NOT EXISTS body_cn TEXT;`）。
- Item 结构加 `BodyCN *string`。**完全照 `reason` 列的既有模式**（body_cn 只在详情页需要，列表不用）：
  - `write.go`：`itemColumns` 末尾加 `body_cn`；Upsert 占位 `$N` +1、`body_cn=EXCLUDED.body_cn`、args 末尾加 `it.BodyCN`；`scanItem`（GetByID 全列扫描）末尾加 `&it.BodyCN`。
  - `query.go`：`listColumns` 末尾加 `NULL::text AS body_cn`（列表不查真值），list 扫描随之加 `&it.BodyCN`（恒为 nil）。
  - 即：详情页 `GetByID` 拿到真 body_cn，列表拿到 nil。与 reason 一致。

### 翻译单元（新文件 `pipeline/translate.go`）
- `Translator` 接口 + 实现，复用现有 `llm.Client`（自带 429 重试/退避）。
- `func Translate(ctx, body string) (string, error)`：system prompt 指示"把下面的 HTML 正文翻成中文，**保留所有 HTML 标签、图片、链接不动，只翻译文字内容**；只输出翻译结果，不要解释"。body 短，单次调用即可；超长（>8000 字）先 `truncateRunes` 截断到安全长度再翻。
- 输出直接作为 `body_cn`（仍是 HTML），详情页渲染时照走 `renderBody` 消毒（沿用 lazy/no-referrer 图片处理）。

### 流水线接入（`pipeline/processor.go`）
- `processOne`：enrichment 成功后，若 `r.SourceKind=="rss"` 且 body 非空，调用 `Translate`，成功则 `it.BodyCN=&cn`。
- **最佳努力**：翻译失败只记日志、不丢条目（详情页回退英文原文）。与 media backfill 同款容错——enrichment 失败才丢条目，翻译失败不丢。
- Processor 需要一个 `Translator`（可选，nil 关闭翻译）；pulse 装配时注入。

### 一次性回填（新 cmd `cmd/backfilltranslate`）
- 找 `source_kind='rss' AND body IS NOT NULL AND body<>'' AND body_cn IS NULL` 的条目，逐条翻译 + `UPDATE items SET body_cn=$1`。仿 `cmd/backfillreason` 结构。
- 在 Melos 上跑一次（228 条，~15 分钟走 RPM）。

### 详情页展示（`detailpage/page.go`）
- viewModel：
  - `Body template.HTML` = 默认展示的正文：rss 且有 body_cn → 渲染 body_cn（中文）；否则渲染原 body。
  - `BodyOriginal template.HTML` = 英文原文（仅 rss 且有 body_cn 时非空）。
  - `HasTranslation bool` = 上面二者都在时为 true。
- 模板：`HasTranslation` 时，中文正文块 + 隐藏的英文原文块 + 一个切换按钮（"看英文原文" ⇄ "看中文翻译"）。两块都在 DOM 里，纯内嵌 JS 切 `display`（仿主题切换的内联 script），**无需前端 fetch**。
- 非 rss（mp）：只有一块，无按钮，行为不变。
- 导出 Markdown（`buildMarkdown`）：rss 有 body_cn 时用中文正文（附一句注明"（正文由 AI 翻译）"），否则原文。

### 错误/边界
- 翻译返回空 / 报错 → body_cn 不写，详情页出英文原文，按钮不出现。
- body 含图片/链接：prompt 要求保留标签；renderBody 消毒后照常渲染。
- 超长 body：翻译前 `truncateRunes` 到安全上限（如 8000 runes），避免超 token。

### 测试
- `translate_test`：prompt 含"保留 HTML 标签/只翻译文字"关键指示；`Translate` 用 fake LLM 返回中文 → 原样透传；LLM 报错 → 返回 error。
- `processor_test`：sourceKind=rss + fake translator → BodyCN 被设；translator 报错 → BodyCN 为 nil 且条目仍入库（best-effort）；sourceKind=mp → 不调翻译、BodyCN nil。
- `detailpage/page_test`：rss+body_cn → vm.HasTranslation=true、Body=中文、BodyOriginal=英文；mp → HasTranslation=false；模板含切换按钮（HasTranslation 时）。
- `items` write/scan round-trip 带 body_cn。

---

## 部署
- schema：`ALTER TABLE items ADD COLUMN IF NOT EXISTS body_cn TEXT`（生产 aihot 执行；新列可空，无需回填约束）。
- 编译发：server（详情页 + 列结构）、pulse（翻译接入）；web bundle（移动端 CSS）。
- 跑一次 `backfilltranslate`（228 条）。
- 验证：手机窄屏核对导航/正文；打开一条英文源详情页 → 默认中文、按钮能切回英文；mp 详情页无按钮、不变。

## Out of scope / Follow-up
- 语言自动检测（现用 source_kind 判定）。
- 列表卡片上的正文翻译（卡片本就只展示中文标题/摘要，无需动）。
- ③ 知识图谱/词云（另立项目）。
- 移动端更深的重设计（本次只做"能用"的响应式修复，不做手机专属交互）。
