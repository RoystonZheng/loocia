# AI Cool 信息源扩展技术方案

| 项目 | 内容 |
|---|---|
| 状态 | 交付稿 |
| 日期 | 2026-09-16 |
| 需求来源 | Cooper 文档《AI Cool 升级方案》第 4 部分 |
| 对应 PRD | [`ai-cool-source-expansion-prd.md`](./ai-cool-source-expansion-prd.md) |
| 代码口径 | `main@4b66d599ddeae24859ac7a159cd3410a2cc2b04b` |
| 范围 | 信息源扩展、来源角色、AIHOT 补漏、精选门槛、前端来源筛选、验收用例 |

## 1. 背景与目标

本次改造解决两个问题：AI Cool 当前资讯覆盖不足，以及部分内容只靠阅读价值分会误入精选。方案在保留现有资讯主链路的前提下扩展来源，并把“采集方式”和“来源可信/使用口径”拆成两个独立概念。

目标链路：

```text
来源配置
  -> RSS / Atom / HTML / 公众号语料 / AIHOT 补漏
  -> RawItem(source_kind + source_role)
  -> raw_items 持久化
  -> enrichment prompt 注入来源名称和来源角色
  -> selected = relevance >= 4 && score >= 3
  -> items / daily / hot topics / graph 继续消费既有字段
  -> Feed 按 source_kind 筛选展示
```

本期不做历史数据回刷，不新增文章状态，不引入资讯管理后台，不继承 AIHOT 的分数和精选结论。

## 2. 方案选择

### 2.1 推荐方案：扩展现有 pulse + ingest + pipeline 链路

沿用当前 `pulse` 定时采集入口，在 `server/internal/pulse` 增加来源配置工厂，在 `server/internal/ingest` 增加不同 adapter，在 `raw_items` 保留 `source_role`，在 `pipeline` 用双门槛决定精选。

选择原因：

- 现有链路已经包含 raw 入队、LLM 富化、items 入库和下游消费，扩展成本最低。
- 来源失败可以继续沿用 `Runner` 的单源错误隔离，不影响其他来源。
- 公共接口只扩展查询参数枚举，不破坏已有前端和下游接口。
- `source_role` 只服务富化阶段，不把内部判断口径暴露给终端用户。

### 2.2 未采用方案：单独建设新采集服务

独立服务会让来源接入更清晰，但本期会增加部署、监控、数据库写入和任务调度成本。当前需求没有要求独立扩缩容，也没有新增复杂审核状态，因此不采用。

### 2.3 未采用方案：把 AIHOT 作为权威精选源

AIHOT 适合补漏发现，但它的分数、精选和摘要不是 AI Cool 的最终结论。直接继承会导致口径不一致，也不满足“AI Cool 自己按公共评分标准判断”的要求，因此只使用它的原文链接、标题、摘要和来源信息作为候选输入。

## 3. 系统架构

### 3.1 模块边界

| 模块 | 代码位置 | 职责 |
|---|---|---|
| 来源配置 | `server/internal/pulse/sources.go` | 读取内置或外部 JSON 配置，校验 adapter/kind/role，实例化 `ingest.Source` |
| 采集 adapter | `server/internal/ingest` | RSS/Atom、Anthropic HTML、公众号语料、AIHOT API v1 转为 `RawItem` |
| raw 存储 | `server/internal/ingest/store.go`、`schema.sql` | 写入 `raw_items`，持久化 `source_kind` 和 `source_role` |
| 富化与精选 | `server/internal/pipeline` | prompt 注入来源上下文，按双门槛生成 `items.selected` |
| 公共 API | `server/internal/publicapi` | `/api/public/items` 支持 `source_kind=rss/html/mp/aihot` |
| 前端 Feed | `web/src/components/Feed.tsx`、`web/src/api/items.ts` | 展示来源筛选 chip，按 `source_kind` 请求列表 |
| 验收套件 | `web/e2e/source-expansion-cases.spec.ts` | 覆盖后端闭环、前端筛选、外部源 smoke、真实 DB 迁移 |

### 3.2 数据流

```mermaid
flowchart TD
  A[pulse.LoadSources] --> B{SourceConfig}
  B --> C1[RSSSource]
  B --> C2[AnthropicNewsSource]
  B --> C3[MPCorpusSource]
  B --> C4[AIHOTSource]
  C1 --> D[RawItem]
  C2 --> D
  C3 --> D
  C4 --> D
  D --> E[RawStore raw_items]
  E --> F[Processor]
  F --> G[Enricher prompt with source role]
  G --> H[toItem double gate]
  H --> I[items]
  I --> J[/api/public/items]
  J --> K[Feed source chips]
```

## 4. 数据模型与迁移

### 4.1 RawItem

`RawItem` 增加 `SourceRole`：

```go
type RawItem struct {
    Source     string
    SourceKind string // rss | html | mp | aihot
    SourceRole string // official | professional | discovery
    URL        string
    Title      string
}
```

`source_kind` 表示采集方式，用于前端筛选和处理分支；`source_role` 表示富化时的阅读价值判断口径，只用于 raw 和 enrichment 阶段。

### 4.2 raw_items

`raw_items` 新增字段和约束：

```sql
ALTER TABLE raw_items
  ADD COLUMN IF NOT EXISTS source_role TEXT NOT NULL DEFAULT 'discovery';

ALTER TABLE raw_items
  ADD CONSTRAINT raw_items_source_role_check
  CHECK (source_role IN ('official','professional','discovery'));
```

实际迁移通过 `DO $$ ... IF NOT EXISTS ... END $$` 包裹约束创建，保证空库和已有表都可重复执行。

### 4.3 items

`items` 不新增字段。新处理的文章继续写入既有字段：

- `source_kind`：允许 `rss`、`html`、`mp`、`aihot`。
- `ai_relevance`：保存 LLM 返回的 AI 相关度。
- `score`：保存 LLM 返回的阅读价值分。
- `selected`：由程序双门槛决定。
- `ai_selected`：与本期双门槛同步，避免保留旧口径。

历史 `items.selected` 不批量回刷。

### 4.4 去重

`RawID` 基于规范化 URL 生成。规范化规则：

- scheme 和 host 小写。
- 去掉默认端口。
- 去掉 fragment。
- 去掉常见追踪参数。
- query 排序。
- 清理 path 中的 `.` 和 `..`。
- `aihot:<id>` 这类稳定非 URL seed 保持原样。

AIHOT 有原文链接时使用原文 URL 作为 ID 种子；无原文链接时只在二手线索上限内使用 `aihot:<id>`。

## 5. 来源配置方案

### 5.1 SourceConfig

配置字段：

| 字段 | 说明 |
|---|---|
| `name` | 来源展示名，必填 |
| `url` | 入口 URL，必填 |
| `adapter` | `rss`、`anthropic_news_html`、`aihot_v1_items`、`mp_corpus` |
| `source_kind` | `rss`、`html`、`aihot`、`mp`，需和 adapter 匹配 |
| `source_role` | `official`、`professional`、`discovery` |
| `enabled` | 是否启用；默认启用 |
| `max_items_per_run` | 单轮最多入队条数 |
| `max_secondary_per_run` | AIHOT 无原文二手线索上限 |
| `timeout_seconds` | 单来源请求超时 |
| `window` | AIHOT 拉取窗口，默认 `7d` |
| `rate_limit` | 单来源抓取前等待，支持 `250ms`、`2s`、`10/m`、`1/5s` |

旧格式 `{name, url}` 继续可用，默认按 `adapter=rss`、`source_kind=rss`、`source_role=discovery` 处理。

### 5.2 内置来源

默认来源包含原有轻量 RSS 源，并新增首批信息源：

| 来源 | adapter | source_kind | source_role | 默认 |
|---|---|---|---|---|
| OpenAI Blog | `rss` | `rss` | `official` | 启用 |
| TechCrunch AI | `rss` | `rss` | `professional` | 启用 |
| Simon Willison | `rss` | `rss` | `professional` | 启用 |
| MIT Technology Review AI | `rss` | `rss` | `professional` | 启用 |
| Anthropic News | `anthropic_news_html` | `html` | `official` | 启用 |
| Google DeepMind | `rss` | `rss` | `official` | 启用 |
| Cloudflare AI | `rss` | `rss` | `official` | 启用 |
| NVIDIA Generative AI | `rss` | `rss` | `official` | 启用 |
| 人人都是产品经理 | `rss` | `rss` | `professional` | 启用 |
| 极客公园 | `rss` | `rss` | `professional` | 启用 |
| Reddit LocalLLaMA | `rss` | `rss` | `discovery` | 启用 |
| Reddit MachineLearning | `rss` | `rss` | `discovery` | 默认禁用 |
| AIHOT | `aihot_v1_items` | `aihot` | `discovery` | 启用 |

Reddit MachineLearning 因本地验证可能返回 429，保留配置但默认禁用，部署机验证后再启用。

### 5.3 Adapter 处理规则

RSS/Atom：

- 复用 `gofeed`。
- 透传 `source_kind`、`source_role`、`max_items_per_run` 和 `timeout`。
- 条目无 link 时跳过。
- 图片、视频提取沿用现有逻辑。

Anthropic HTML：

- 只解析 Anthropic News 列表页。
- 抽取标题、链接和可用日期。
- 不递归抓全站，正文仍交给现有 PageResolver 尽力补抓。
- HTML 结构变化时该来源报错，其他来源继续。

公众号语料：

- 继续读取 `AIHOT_MP_CORPUS_DIR` 下的本地 Markdown。
- 默认 `source_role=professional`。
- `AIHOT_MP_OFFICIAL_ACCOUNTS` 命中 frontmatter 中的账号名时改为 `official`。
- 不新增微信直连接口。

AIHOT API v1：

- 默认请求 `/api/v1/items?mode=all&window=7d&by=published&limit=50`。
- 有 `links.original` 时优先使用原文链接。
- 无原文链接时使用 `links.aihot`，来源展示为 `AIHOT · 二手线索`，并受 `max_secondary_per_run` 限制。
- `title`、`originalTitle`、`summary` 只作为 raw 输入。
- 不使用 AIHOT 的 `score`、`selected`、`reason` 作为 AI Cool 结论。
- 不调用未确认的详情接口，不抓 AIHOT 网页绕过授权边界。

## 6. 富化与精选策略

### 6.1 来源角色

| source_role | 判断口径 |
|---|---|
| `official` | 官方博客、官网新闻、明确官方公众号；官方身份不自动加分，活动、招聘、空洞宣传仍低分 |
| `professional` | 科技媒体、专家博客、普通公众号；重视原创采访、实测、代码、数据和独特分析 |
| `discovery` | 社区、聚合和补漏来源；传闻、截图、单一反馈、二手摘要要压低 |

### 6.2 Prompt

`server/internal/pipeline/enrichment.go` 在 prompt 中加入：

```text
来源名称：...
来源角色：official | professional | discovery
原始标题：...
正文：...
```

系统提示词保留现有六项结构化输出，同时补充三类来源角色的阅读价值判断规则。模型继续返回：

- `title_cn`
- `summary_cn`
- `category`
- `relevance`
- `score`
- `reason_cn`

### 6.3 精选门槛

程序最终门槛：

```go
selected := relevance >= 4 && score >= 3
aiSelected := selected
```

边界：

| relevance | score | selected |
|---:|---:|---|
| 4 | 3 | true |
| 5 | 5 | true |
| 3 | 5 | false |
| 5 | 2 | false |
| 1 | 5 | false |
| 4 | 1 | false |

### 6.4 队列优先级与富化容错

`RawStore.ListUnprocessed` 对未处理队列按来源做轻量优先级排序：`source_kind=aihot` 先于普通 RSS/HTML/MP backlog。原因是 AIHOT 是补漏源，如果严格按最老 `fetched_at` 处理，历史 RSS 未处理队列会挡住当日 AIHOT 内容，导致 `/api/public/items?source_kind=aihot` 长时间为空。

`Processor` 对单条富化做以下保护：

- 单条处理有总超时，避免一次 LLM/页面/抽词调用拖住整轮 `pulse`。
- AIHOT 不追溯原网页做 PageResolver 补正文，直接使用 AIHOT API 给出的标题、摘要和原文链接进入富化，避免补漏链路被外部页面抓取拖慢。
- 模型 `category` 允许常见别名归一，如把 `discovery` 归到 `industry`，但无意义分类仍报错。
- 模型输出多个 JSON 或 JSON 后带多余内容时，只解析第一个合法 JSON 对象，降低单条富化失败率。

## 7. 公共接口方案

### 7.1 `GET /api/public/items`

接口继续匿名只读，响应结构不新增必填字段。

请求参数：

| 参数 | 取值 | 说明 |
|---|---|---|
| `mode` | `selected`、`all` | 默认 `selected` |
| `category` | 既有五类 | 分类筛选 |
| `source_kind` | `rss`、`html`、`mp`、`aihot` | 来源类型筛选；未知值返回 400 |
| `q` | string | 至少 2 个字符才生效 |
| `take` | 1-100 | 默认 50 |
| `cursor` | opaque cursor | 翻页 |

响应：

```json
{
  "count": 1,
  "hasNext": false,
  "nextCursor": null,
  "items": [
    {
      "id": "item-id",
      "title": "中文标题",
      "url": "https://example.com/article",
      "permalink": "/items/item-id",
      "source": "AIHOT · 二手线索",
      "selected": true
    }
  ]
}
```

`PublicItem` 不暴露 `source_role`、`ai_relevance`、`ai_selected` 等内部字段。

### 7.2 兼容性

- 不带 `source_kind` 的旧请求行为不变。
- `source_kind=mp` 浏览历史公众号语料时不加默认 7 天窗口。
- `rss/html/aihot` 仍使用默认窗口。
- 搜索 `q` 继续跨全量归档。
- 日报、热点、图谱继续消费 `items` 既有字段，无需改接口。

## 8. 前端展示方案

### 8.1 页面范围

不新增页面路由。继续使用：

```text
#selected  精选
#all       全部 AI 动态
#daily     AI 日报
#graph     图谱
```

### 8.2 来源筛选

`Feed` 顶部来源筛选为：

| 文案 | 请求 |
|---|---|
| 全部来源 | 不传 `source_kind` |
| RSS/Atom | `source_kind=rss` |
| 网页直采 | `source_kind=html` |
| 公众号 | `source_kind=mp` |
| AIHOT补漏 | `source_kind=aihot` |

点击来源、分类或搜索提交后会重新加载列表并重置 cursor。

### 8.3 卡片展示

卡片继续展示标题、来源、发布时间、分类、score、精选标记、摘要、图片和视频。`source_role` 不展示，避免用户把内部评分口径理解为质量背书。

AIHOT 二手线索通过 `source="AIHOT · 二手线索"` 表达来源性质，不新增警告卡片。

### 8.4 状态

- 接口失败：展示“加载失败，请稍后重试。”
- 筛选后无结果：展示“暂无资讯。”
- 加载更多：沿用当前按钮和禁用态。

## 9. 异常处理与降级

| 场景 | 处理 |
|---|---|
| 配置文件不存在或格式错误 | `pulse` 启动失败，避免静默使用错误配置 |
| 未知 adapter/kind/role | `LoadSources` 返回错误 |
| `enabled=false` | 跳过该来源 |
| 单个来源网络失败 | 记录 source error，其他来源继续 |
| 429/503 | adapter 返回可观察错误；来源可配置 `rate_limit` 降低频率 |
| HTML 结构漂移 | Anthropic 来源本轮失败，不影响其他来源 |
| AIHOT 无原文链接 | 只在二手线索上限内保留 |
| AIHOT 被旧队列阻塞 | 未处理队列优先处理 `source_kind=aihot` |
| 单条富化耗时过长 | 单条超时后记录 failed，其他条目继续 |
| LLM 输出 `category=discovery` 等别名 | 归一到既有五类，无法识别的分类仍失败 |
| LLM 在 JSON 后追加多余内容 | 只解析第一个合法 JSON 对象 |
| DB 迁移重复执行 | `ADD COLUMN IF NOT EXISTS` + 约束存在性检查保证幂等 |
| 旧配置只有 name/url | 兼容为 RSS + discovery |

## 10. 部署与回滚

### 10.1 上线步骤

1. 确认目标环境数据库已可连接。
2. 放置外部来源配置文件，例如 `/root/aihot/etc/sources.json`。
3. 设置 `AIHOT_SOURCES_FILE`，使 `deploy/aihot-cron.sh` 的 `pulse` 使用外部配置。
4. 如启用公众号语料，设置 `AIHOT_MP_CORPUS_DIR`。
5. 如需要官方公众号覆盖，设置 `AIHOT_MP_OFFICIAL_ACCOUNTS`。
6. 手动执行一次 `pulse` smoke。
7. 验证 `/api/public/items?mode=all&source_kind=aihot` 和前端筛选。
8. 观察 `pulse` 日志中的 fetched、inserted、srcerrs 及具体的 `source error: <来源>` 明细。

### 10.2 回滚策略

代码回滚：

- 回滚到上一版本后，新增 `raw_items.source_role` 字段可保留，不影响旧代码读取。
- 公共 API 不新增响应字段，前端旧版本可继续消费。

配置回滚：

- 移除或清空 `AIHOT_SOURCES_FILE` 可退回内置默认来源。
- 将高风险来源设为 `enabled=false` 即可停用单源。
- Reddit、AIHOT 等外部波动源优先通过配置停用，不需要重新发布。

数据回滚：

- 不建议批量删除已入库资讯。
- 若某来源误入大量低质量内容，可按 `source_kind`、`source`、时间窗口人工排查处理。
- 历史 `selected` 不回刷；如需批量重评，应作为新需求处理。

## 11. 验收方案

### 11.1 自动化测试

| 层级 | 命令 | 结论 |
|---|---|---|
| Web lint | `cd web && npm run lint` | 通过 |
| Web 类型检查 | `cd web && npx tsc -p tsconfig.node.json --noEmit` | 通过 |
| Playwright 信息源套件 | `AICOOL_LIVE_SOURCE_TESTS=1 AIHOT_TEST_DATABASE_URL=... npx playwright test --project=desktop-chrome e2e/source-expansion-cases.spec.ts` | `4 passed` |
| 真实 DB 迁移 | `TC-E2E-004` | 通过，`source_role` 字段和约束生效 |

Playwright 覆盖：

- 后端闭环：`pulse`、`ingest`、`pipeline`、`publicapi` 相关 Go 测试。
- 前端筛选：全部/RSS/HTML/MP/AIHOT、错误态、`source_role` 不展示。
- 外部源可达性：Anthropic、DeepMind、Cloudflare、NVIDIA、人人都是产品经理、极客公园、Reddit、AIHOT。
- PostgreSQL schema：空库/已有表迁移。

### 11.2 数据库验收点

```sql
SELECT column_name, is_nullable, column_default
FROM information_schema.columns
WHERE table_name='raw_items' AND column_name='source_role';
```

期望：

```text
source_role | NO | 'discovery'::text
```

```sql
SELECT conname, pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conrelid='raw_items'::regclass
  AND conname='raw_items_source_role_check';
```

期望约束：

```text
CHECK source_role IN ('official','professional','discovery')
```

### 11.3 端到端验收清单

| Case ID | 验收点 | 预期 |
|---|---|---|
| TC-SRC-001 | 默认来源加载 | 返回内置来源，角色明确 |
| TC-SRC-002 | 旧 JSON 配置 | `{name,url}` 兼容为 RSS + discovery |
| TC-SRC-003 | 非法配置 | 未知 adapter/kind/role 直接失败 |
| TC-ADP-001 | RSS 入队 | RawItem 带 `rss` 和配置角色 |
| TC-ADP-002 | Anthropic HTML | 抽取新闻列表，写 `source_kind=html` |
| TC-ADP-003 | MP 官方账号 | 命中官方账号后 `source_role=official` |
| TC-ADP-004 | AIHOT 原文 | 使用 `links.original` 去重入队 |
| TC-ADP-005 | AIHOT 二手线索 | 使用 `AIHOT · 二手线索`，受上限限制 |
| TC-DEDUP-001 | URL 规范化 | 常见追踪参数、fragment、默认端口不影响去重 |
| TC-PIPE-001 | Prompt 来源上下文 | prompt 包含来源名称和来源角色 |
| TC-PIPE-002 | 双门槛通过 | relevance=4, score=3 进入精选 |
| TC-PIPE-003 | 相关度不足 | relevance=3, score=5 不进精选 |
| TC-PIPE-004 | AIHOT 队列优先 | AIHOT raw 不被旧 RSS backlog 长时间阻塞 |
| TC-PIPE-005 | AIHOT 不追溯页面 | AIHOT 富化不调用 PageResolver 抓原文 |
| TC-PIPE-006 | LLM 输出容错 | category 别名归一，尾部多余 JSON 文本不丢整条 |
| TC-API-001 | `source_kind=html` | 返回网页直采内容 |
| TC-API-002 | `source_kind=aihot` | 返回 AIHOT 补漏内容 |
| TC-API-003 | 非法 `source_kind` | 返回 400 |
| TC-FE-001 | 来源 chip 展示 | 五个筛选项可见 |
| TC-FE-002 | 点击筛选 | 请求带正确 `source_kind` 并重置列表 |
| TC-FE-003 | 内部字段隐藏 | 页面不展示 `source_role` |
| TC-FE-004 | 错误态 | 接口失败展示“加载失败，请稍后重试。” |
| TC-E2E-003 | 外部源 smoke | 默认外部源当前可达或按允许状态返回 |
| TC-E2E-004 | 真实 PostgreSQL 迁移 | 空库/已有表均迁移成功 |

## 12. 需求覆盖矩阵

| 需求 | 方案覆盖 | 代码/文档位置 |
|---|---|---|
| 扩大信息源覆盖 | 新增 RSS、HTML、公众号语料、AIHOT 四类来源 | `server/internal/pulse/sources.go`、`server/internal/ingest/*` |
| 区分采集方式和来源角色 | `source_kind` + `source_role` 双字段 | `server/internal/ingest/rawitem.go`、`schema.sql` |
| 来源角色传给 AI | enrichment prompt 注入来源名称和角色规则 | `server/internal/pipeline/enrichment.go` |
| 精选同时看相关度和价值 | `relevance >= 4 && score >= 3` | `server/internal/pipeline/processor.go` |
| AIHOT 只做补漏 | 只使用公开 items 字段，不继承分数和精选 | `server/internal/ingest/aihot.go` |
| 单源失败不阻断全链路 | Runner 收集 source errors 后继续 | `server/internal/ingest/runner.go` |
| 前端按来源筛选 | Feed source chips 调 `/api/public/items?source_kind=...` | `web/src/components/Feed.tsx`、`web/src/api/items.ts` |
| 接口兼容 | 响应不新增必填字段，只扩展查询枚举 | `docs/api/items.md`、`server/internal/publicapi/params.go` |
| 不回刷历史 | 只改变新处理 items 的 selected | `processor.go` 和上线说明 |
| 可验收 | Playwright 覆盖后端、前端、外部源、真实 DB | `web/e2e/source-expansion-cases.spec.ts` |

## 13. 风险与后续

| 风险 | 当前控制 | 后续建议 |
|---|---|---|
| 外部源限流或不可达 | `enabled`、`rate_limit`、单源错误隔离 | 生产观察 `srcerrs`，必要时按来源停用 |
| HTML 结构变化 | Anthropic adapter 失败不影响其他来源 | 增加监控或定期 smoke |
| AIHOT 授权边界 | 只用公开只读 API，不抓网页正文 | 对外再分发前补授权确认 |
| LLM 评分波动 | 程序双门槛兜底，prompt 固定来源角色规则 | 后续可做抽样人工评估 |
| 测试库并发污染 | Playwright 调度 Go 多 package 测试时使用 `-p=1` | CI 若并发跑 DB 测试，优先每包独立库 |
