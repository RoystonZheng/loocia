# 全文抓取（RSS 原文显示不全）— Design

**Date:** 2026-07-14
**Status:** Approved (co-designed with user)
**Goal:** RSS 源目前只存 feed 给的一小段摘要（~82% 不足 500 字），详情页"原文"因此不全。抓源 URL + readability 提取正文，把 `body` 换成全文，让原文完整、并给翻译提供全文上下文。

**Phase 关系**：这是 Phase 1。做完后 Phase 2（`2026-07-14-translation-quality-and-detail-ux.md`：校验/重试/返回键/声明模型）在**全文**上重跑翻译。两阶段合并一次部署、一次回填链（先抓全文 → 再重译）。

## 背景（实测）
- rss `body` 中位 165 字、~82%(188/229) 不足 500 字、仅 7 条 >3000；不是渲染截断，是 feed 只给导语。
- 全文抓取同时**改善翻译质量**：16 条"回英文"多半因模型只拿到一句碎片。
- mp（公众号）本就全文，不动。

## 依赖
- 新增 `github.com/go-shiori/go-readability`（Mozilla Readability 的 Go 移植，内网 goproxy 可达，已验证 `v0.0.0-20251205110129-...`）。
- `github.com/PuerkitoBio/goquery`、`golang.org/x/net` 已是依赖。

## 组件

### 1. PageResolver（抓一次页面，同时出 OG 媒体 + 正文）
现状：`ingest.OGResolver` 实现 `MediaResolver.Resolve(ctx,url)→(image,video *string)`，只扫 `<head>` 取 OG。
改：升级成**抓整页一次**（有 size cap），既解析 OG（现有正则，head 在整页里）又跑 readability 提正文。

- `ingest/article.go`（新）：`func extractArticle(htmlBytes []byte, pageURL string) (string, bool)`——`readability.FromReader(bytes.NewReader(htmlBytes), parsedURL)`，返回 `Article.Content`（清洗后的正文 HTML）；解析失败/内容过短返回 `("", false)`。
  - **实现前先确认 go-readability 实际 API**：该库不同版本 `FromReader` 第二参可能是 `*url.URL` 或 `string`，`Article` 字段名以实际为准（`.Content` / `.TextContent`）。`go doc github.com/go-shiori/go-readability` 查一下再写。
- `ingest/ogfetch.go` 改造：新增 `PageResolver`，`Resolve(ctx, url) PageResult`，`PageResult{ Image, Video, ArticleHTML *string }`。内部：一次 GET（cap 3MB、原有超时/UA/best-effort），拿 `htmlBytes` → OG 正则出 image/video + `extractArticle` 出正文。
- 抓取上限：`maxPageBytes = 3<<20`（`io.LimitReader`）。抓取失败/超时/非 200 → 全空，best-effort。

### 2. processor 用全文（`pipeline/processor.go`）
`MediaResolver` 接口升级为 `PageResolver` 接口：`Resolve(ctx, pageURL string) (image, video, article *string)`（三返回）。processOne：
```
if p.page != nil {
    img, vid, article := p.page.Resolve(ctx, r.URL)
    // 媒体回填（同现状：仅在缺失时补）
    if it.ImageURL == nil { it.ImageURL = img }
    if it.VideoURL == nil { it.VideoURL = vid }
    // 全文替换：仅 rss，且提取正文明显比 snippet 长才用
    if r.SourceKind == "rss" && article != nil && betterBody(*article, it.Body) {
        it.Body = article
    }
}
```
- `betterBody(extracted string, cur *string) bool`：提取正文**去标签后文本长度 ≥ 300 且 ≥ 现有 snippet 文本长度 × 1.5**（避免用更差的提取覆盖尚可的 snippet）。
- 顺序：enrich（仍基于 snippet，够生成中文标题/摘要/分类/评分）→ toItem → **page resolve（可能换 body 成全文）** → 翻译（Phase 2 校验，跑在全文上）→ upsert。
- 命名：Processor 字段 `media` → `page`；`WithMediaResolver` → `WithPageResolver`（或保留旧名加新方法，取**改名**更清晰，同步改 pulse/测试）。

### 3. 回填全文（`cmd/backfillarticle`，新）
仿 backfill* 结构。对 `source_kind='rss'` 且 body 文本短（如 <500 字）的 item：抓 URL、提取、`betterBody` 通过则 `UPDATE items SET body=$1, body_cn=NULL, body_cn_model=NULL, updated_at=now()`（置空 body_cn 让 Phase 2 在全文上重译）。best-effort，抓不到就跳过留 snippet。打印 targets/updated/skipped。
- 纯 HTTP、不吃 LLM RPM，可快些；但对源站礼貌：串行 + 每条间隔小睡（或并发很低），避免被封。

### 4. pulse 装配 / cmd/pulse
`pulse.Deps` 的 resolver 从 OGResolver 换成 PageResolver；`cmd/pulse` 构造 `ingest.NewPageResolver()`。

## Error handling / 边界
- 抓取任何失败（网络/403/付费墙/超时/非 HTML）→ 三返回空，best-effort，item 保留 snippet body。
- readability 提取出的内容仍要过 `detailpage.renderBody` 的 bluemonday 消毒（现状已消毒，图片加 lazy/no-referrer）。
- 提取正文可能含源站导航残留——readability 已尽力去噪；不额外处理，够用。
- 大页面 3MB cap；单请求超时沿用现有 client 超时。

## 测试
- `article_test`：给一段含 `<article>`+噪声的 HTML，`extractArticle` 提出正文、去掉 nav/script；无正文/空 → ok=false。
- `ogfetch/page_test`：PageResult 同时含 OG 媒体 + 正文（用本地 HTML fixture / httptest server）。
- `processor_test`：rss + PageResolver 返回长正文 → it.Body 被替换；正文太短 → 保留 snippet；mp → 不替换。`betterBody` 单测边界（300 字门槛、1.5×）。
- 不真联网（用 httptest / fixture）。

## 部署（与 Phase 2 合并一次）
- `go mod tidy` 拉 go-readability（内网 goproxy）；确认 `make test` + 交叉编译能拉到依赖（Melos 编译在**本地 Mac** 交叉编译，用本机 goproxy，无需 Melos 联网）。
- 加列 `body_cn_model`（Phase 2）。
- 发 server+pulse+backfillarticle+backfilltranslate+web。
- 回填链：先 `backfillarticle`（抓全文、置空 body_cn）→ 再 `backfilltranslate`（全文上重译，带 Phase 2 校验）。
- 验证：一条原本只有一句的 rss 详情页 → 现出完整正文；翻译基于全文、质量抽检。

## Out of scope
- 对付费墙/强反爬站做特殊绕过（best-effort，抓不到留 snippet）。
- 重跑 enrichment（摘要/评分仍基于 snippet，够用）。
- 非 rss 源的全文（mp 本就全文）。
