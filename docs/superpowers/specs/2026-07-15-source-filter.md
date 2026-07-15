# Feed 来源筛选（公众号入口）— Design

**Date:** 2026-07-15
**Status:** Approved (co-designed with user)
**Goal:** 让 748 篇公众号（mp）历史文章可被浏览。现在浏览 feed 有 7 天新鲜度地板（`sinceWindow`），mp 文章多为数月前发布 → 被过滤掉，只有搜索才能看到。加一个来源筛选，切到「公众号」时放开时效窗口。

## 背景（实测）
- items 两类来源：`rss`（英文源，发布日期新）、`mp`（公众号，10 个大厂号 748 篇，发布日期跨 2024–2026）。
- `params.go` 浏览时套 `sinceWindow = 7*24h` 地板；mp 仅 9 篇落在近 7 天 → 默认 feed 几乎看不到 mp，被 rss 淹没。
- mp 内容已入库、已富集、586 篇精选——不是没进，是被时效窗口挡住。

## 用户选择
- **入口形态**：在现有 feed 顶部加**来源切换**（不是独立侧边栏页）。
- **默认范围**：跟随现有 mode（精选/全部）。精选 mode + 公众号 → 只看精选公众号（586 篇里的精选）。

## 组件

### 1. 来源切换 UI（`web/src/components/Feed.tsx`）
在 `feed-controls` 里、分类 chips 旁/上，加一组来源 chips：
- `全部来源`（默认，`sourceKind=undefined`）、`公众号`（`mp`）、`英文源`（`rss`）。
- 复用现有 `.chip`/`.chip.active` 样式；新加一行 `.feed-sources`（与 `.feed-cats` 并列）。
- state `sourceKind: 'mp' | 'rss' | undefined`；进 `load` 的 useCallback 依赖；传给 `fetchItems`。
- 分类筛选、搜索与来源筛选正交，可叠加。

### 2. API 参数（`web/src/api/items.ts`）
`ListQuery` 加 `sourceKind?: 'mp' | 'rss'`；`fetchItems` 里 `if (query.sourceKind) params.set('source_kind', query.sourceKind)`。

### 3. 后端参数解析（`server/internal/publicapi/params.go`）
- 解析 `source_kind`：只允许 `mp` / `rss`，其它 → 400。存 `p.SourceKind *string`。
- **时效窗口放开**：`archive := p.SourceKind!=nil && *p.SourceKind=="mp"`；`allTime := isSearch || archive`。把原来 `!isSearch` 的两处判断（`since` 参数 clamp、默认 `p.Since=&lower`）都改成 `!allTime`。即：切到公众号时不套 7 天地板，按发布时间倒序看全部历史；rss/全部来源仍套 7 天地板（新闻要新鲜）。

### 4. 后端查询（`server/internal/items/`：ListParams + 查询）
- `items.ListParams` 加 `SourceKind *string`。
- 列表查询 WHERE 加 `AND ($N::text IS NULL OR source_kind = $N)`（照现有 Category 过滤的写法，用同样的 keyset 游标分页）。参数按现有占位符顺序追加。

## Error handling / 边界
- `source_kind` 非法值 → 400（同 category）。
- 全部来源默认行为不变（仍套 7 天窗口）——用户要看公众号需显式切「公众号」，符合“专门看公众号的入口”。
- keyset 游标：来源筛选是 WHERE 增量，不影响 `(published_at, id)` 游标排序；翻页正常。
- 精选/全部 mode 与来源正交：精选 mode 传 `selected=true`，来源 mp → 精选公众号。

## 测试
- `params_test`：`source_kind=mp` → `p.Since==nil`（放开窗口）、`p.SourceKind=="mp"`；`source_kind=rss` → 仍有 7 天 `Since`；非法值 → errBadRequest；无参数 → 行为不变。
- items 查询测试：`SourceKind=mp` 只返回 mp；`=rss` 只返回 rss；`nil` 全返回。
- `Feed.test`：来源 chips 渲染、点击「公众号」触发带 `source_kind=mp` 的 fetch。
- `api/items` 参数拼接（若有覆盖）。

## 部署
- 编译发 server + web（随翻译回填之后一次发；回填是纯数据，与本功能解耦）。
- 验证：切「公众号」→ 出历史大厂号文章（按发布时间倒序）；切「英文源」→ 仅英文；「全部来源」→ 维持现状（近 7 天）。精选 mode 下公众号只出精选。

## Out of scope
- 按公众号账号（阿里云/字节…）二级筛选——本轮不做（用户未选账号分组；YAGNI，后续可加 `source=` 参数）。
- 独立侧边栏「公众号」页。
- 改 mp 的富集/评分。
