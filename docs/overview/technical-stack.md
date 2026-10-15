# 技术栈

## 口径

本文描述当前代码和当前工作区部署脚本已经实现的技术栈、协议、数据流和运行边界。
“依赖声明”不等于“业务链路正在使用”：例如 `go.mod` 中保留了 Nuwa、Redis、MySQL 和 gRPC 兼容依赖，但当前资讯和工具的主数据路径是 PostgreSQL + `pgxpool`。

主要代码入口：

- 后端 HTTP：`server/cmd/webserver/main.go`
- Nuwa 兼容 HTTP：`server/common/server/httpserv/httpserv.go`
- 资讯采集与处理：`server/internal/ingest`、`server/internal/pulse`、`server/internal/pipeline`
- LLM 客户端：`server/internal/llm/client.go`
- 资讯存储：`server/internal/items`、`server/internal/ingest/store.go`
- 工具发现：`server/internal/tools`、`server/cmd/discovertools`
- 前端入口：`web/src/main.tsx`、`web/src/App.tsx`

## 一句话架构

```text
RSS/Atom、HTML、Markdown corpus、AIHOT API
        │
        ▼
source adapter → HTTP 重试/限流 → 原始资讯 raw_items
        │
        ▼
规范化 URL / SHA-256 ID / 幂等去重
        │
        ▼
正文与媒体补全 → LLM 结构化富化 → 译文与术语抽取
        │
        ▼
items / item_terms → 选中流、日报、热点聚类、词云、图谱
        │
        ▼
Go API → Vite/React 前端；Nginx 对外提供静态资源和反向代理

GitHub REST API → 工具发现与星标快照 → tools 领域表
                                      │
                                      ▼
                           工具筛选、团队池、评价、Cooper URL
```

## 总体技术选型

| 层次 | 当前实现 |
|---|---|
| 前端语言 | TypeScript |
| 前端框架 | React 19 |
| 前端构建 | Vite 8 + TypeScript project build |
| 前端数据访问 | 浏览器原生 `fetch`，没有 Axios、React Query、Redux 或 Zustand |
| 前端可视化 | `d3-cloud`、`d3-force` |
| 后端语言 | Go，`go.mod` 声明 `go 1.25.0` |
| HTTP | 标准库 `net/http`；另有 Nuwa/rpcserver 兼容入口 |
| RPC/IDL 兼容 | Nuwa、gRPC、gRPC-Gateway、生成的 `*.pb.go` / `*_http.go` |
| 主数据库 | PostgreSQL |
| 数据库驱动 | `github.com/jackc/pgx/v5/pgxpool` |
| Schema 管理 | 各领域内嵌 `schema.sql`，启动或命令执行时通过 `EnsureSchema` 幂等初始化 |
| 资讯协议 | RSS/Atom、网页 HTML、Markdown 文件树、AIHOT JSON API |
| 工具协议 | GitHub REST API |
| LLM 协议 | Anthropic Messages 风格的内部 HTTP 代理 |
| 网关/静态资源 | Nginx |
| 常驻进程 | supervisor |
| 定时调度 | cron；本地额外支持 macOS LaunchAgent/cron |
| 后端测试 | Go `testing`，数据库测试串行执行 |
| 前端单测 | Vitest + Testing Library + jsdom |
| 浏览器测试 | Playwright |
| 前端 lint | oxlint |

## 前端

### 框架、依赖和构建

| 维度 | 技术或实现 | 代码依据 |
|---|---|---|
| 运行时 | React `^19.2.7`、React DOM `^19.2.7` | `web/package.json` |
| 语言 | TypeScript `~6.0.2` | `web/package.json`、`web/tsconfig*.json` |
| 构建工具 | Vite `^8.1.1`、`@vitejs/plugin-react` | `web/vite.config.ts` |
| JSX 转换 | `react-jsx` | `web/tsconfig.app.json` |
| 可视化 | `d3-cloud` 词云、`d3-force` 力导向图 | `web/src/components/WordCloud*`、`GraphView*` |
| CSS | 项目自有 CSS，集中在 `web/src/index.css` 等文件 | `web/src` |
| UI 组件库 | 当前未使用 Ant Design、MUI 等大型组件库 | `web/package.json` |
| 状态管理 | React hooks 和组件状态；没有独立状态管理库 | `web/src` |
| 网络请求 | 原生 `fetch` 封装在 `web/src/api/*.ts` | `web/src/api` |
| 路由 | `window.location.hash` + `history.replaceState` | `web/src/App.tsx` |

### 页面和路由

当前前端通过 hash 选择主视图，不使用 React Router：

| Hash | 页面职责 |
|---|---|
| `/` | 日报首页 |
| `#selected` | AI 选中的资讯流 |
| `#all` | 全量资讯流 |
| `#graph` | 术语图谱/词云 |
| `#tools-configs` | 工具发现配置 |
| `#tools-accounts` | GitHub 账号或 Token 配置 |
| `#tools-discovered` | 已发现工具 |
| `#tools-evaluating` | 评价中的工具视图 |
| `#tools-team` | 团队工具池 |

前端主要组件包括：

- `Feed`：资讯列表、筛选、分页和条目跳转
- `DailyView`：日报
- `GraphView`、`WordCloud`、`TermPanel`：术语图谱和词云
- `HotTopics`：热点聚类
- `ToolsView`：工具发现、评价和团队池
- `Sidebar`：主导航和工具数量徽标
- `ItemCard`：资讯卡片

### 开发代理和浏览器兼容

`web/vite.config.ts` 的开发代理将以下请求转发到 Go 服务：

| 前端路径 | 默认后端目标 | 用途 |
|---|---|---|
| `/api` | `http://localhost:8991` | JSON API |
| `/items` | `http://localhost:8991` | 资讯详情 SSR 和详情操作 |

代理目标可通过 `AI_TOOL_WEB_API_PROXY` 覆盖。当前 CSS 构建目标包含 `chrome87`、`safari13.1`、`firefox78` 和 `edge88`，原因是兼容旧版浏览器和部分内置 WebView。

### 前端测试工具链

| 类型 | 工具和配置 |
|---|---|
| 单元/组件测试 | Vitest、Testing Library、`@testing-library/jest-dom` |
| DOM 环境 | jsdom |
| 浏览器 E2E | `@playwright/test` |
| E2E 浏览器 | `Desktop Chrome`，默认 viewport `1440x900` |
| E2E 报告 | list、JSON、HTML；失败保留 trace、截图和视频 |
| E2E 默认端口 | `5176`；可用 `PLAYWRIGHT_BASE_URL` 或 `PLAYWRIGHT_PORT` 覆盖 |
| lint | oxlint |
| 构建前检查 | `npm run build` 先执行 `tsc -b`，再执行 Vite build |

前端没有独立的 BFF 层，也没有在浏览器端直接访问 PostgreSQL、GitHub 或 LLM。所有数据访问都经过后端 API。

## 后端

### 语言、入口和框架

| 维度 | 当前实现 |
|---|---|
| 语言 | Go |
| 模块 | `aihot-server` |
| Go 版本约束 | `go.mod` 为 `1.25.0`；`server/Makefile` 的 `run`、`test`、`build` 目标固定 `GOTOOLCHAIN=go1.25.5` |
| 公共 HTTP 入口 | 标准库 `net/http`、`http.ServeMux`、优雅退出、panic recovery |
| Nuwa HTTP 入口 | `rpcserver/v2`、Nuwa recovery、trace、metrics、JSON marshaler |
| 配置 | 环境变量 + `server/conf/app.toml`；Nuwa 入口使用 Viper |
| 默认监听 | `AIHOT_HTTP_ADDR`，默认 `:8991` |
| 超时 | 公共入口 `ReadTimeout=15s`、`IdleTimeout=60s`；Nuwa 配置默认读超时 5s、空闲超时 60s |
| 数据库连接 | `pgxpool.New(ctx, dsn)` |
| 并发模型 | Go goroutine；资讯源按源串行处理，LLM 和工具 API 使用上下文超时 |

标准 HTTP 入口和 Nuwa 入口注册的是同一组业务 handler，区别在于外围服务框架和中间件。实际部署使用哪个入口，应以 `deploy/aihot-server.sh` 和构建产物为准。

### 主要 Go 依赖

| 依赖 | 用途 |
|---|---|
| `github.com/jackc/pgx/v5/pgxpool` | PostgreSQL 连接池、查询和事务 |
| `github.com/mmcdole/gofeed` | RSS/Atom 解析 |
| `github.com/PuerkitoBio/goquery` | HTML DOM 查询 |
| `github.com/go-shiori/go-readability` | 从原始文章页提取正文 |
| `github.com/microcosm-cc/bluemonday` | HTML 内容清理 |
| `github.com/yuin/goldmark` | Markdown 和 GFM 转 HTML |
| `github.com/spf13/viper` | Nuwa 配置读取 |
| `github.com/grpc-ecosystem/grpc-gateway` | gRPC/HTTP 网关兼容 |
| `google.golang.org/grpc`、protobuf 生成代码 | 内部 RPC/IDL 兼容 |
| `git.xiaojukeji.com/nuwa/golibs/*` | Nuwa HTTP、监控、指标、Redis、日志和 RPC 兼容能力 |
| `golang.org/x/net/html` | HTML 相关底层能力 |

`go.mod` 还声明了 MySQL、Redis、GORM 等兼容依赖。当前资讯、日报、热点、术语和工具的主读写路径均由 PostgreSQL 领域 store 负责；不能仅凭这些间接依赖判断业务正在使用 MySQL 或 Redis。

### 后端模块

| 模块 | 责任 |
|---|---|
| `internal/db` | 创建和关闭 PostgreSQL 连接池 |
| `internal/ingest` | 多来源采集、正文解析、媒体补全、URL 规范化、原始数据落库 |
| `internal/pulse` | 一次采集和富化批次的编排、批量上限、源错误隔离 |
| `internal/pipeline` | LLM 富化、翻译、术语抽取、选中规则和 raw 状态推进 |
| `internal/llm` | Anthropic-compatible LLM HTTP 客户端、重试和模型配置 |
| `internal/items` | `items` 表的 upsert、列表查询、游标分页和详情查询 |
| `internal/terms` | 术语替换、词云和图谱数据 |
| `internal/daily` | 日报分组、快讯、导语生成和日报 upsert |
| `internal/cluster` | LLM 同事件聚类和热点热度计算 |
| `internal/publicapi` | 公共资讯 API、日报 API、热点 API、图谱 API、工具 API |
| `internal/detailpage` | `/items/{id}` 详情页和重新翻译 |
| `internal/tools` | 工具发现配置、GitHub client、工具状态、团队池和评价数据 |
| `cmd/pulse` | 采集和资讯处理任务 |
| `cmd/gendaily` | 生成或重生成日报 |
| `cmd/hotpass` | 热点聚类和热度计算 |
| `cmd/discovertools` | GitHub 工具发现、快照、定时调度和人工运行 |
| `cmd/importmp` | 本地 Markdown/公众号 corpus 导入 |

## 获取资讯协议和采集层

### 来源类型

来源配置由 `server/internal/pulse/sources.go` 解析，核心字段包括：

`id`、`name`、`url`、`adapter`、`source_kind`、`source_role`、`enabled`、`max_items_per_run`、`fetch_limit`、`max_secondary_per_run`、`timeout_seconds`、`retry_attempts`、`window`、`rate_limit`、字段映射和 HTML selector。

当前来源分类：

| 字段 | 可用值 | 作用 |
|---|---|---|
| `adapter` | `rss`、`json_api`、`html_list`、`mp_corpus`、`anthropic_news_html`、`aihot_v1_items` | 选择协议适配器 |
| `source_kind` | `rss`、`html`、`api`、`mp`、`aihot` | 进入 raw/item 后的来源类型 |
| `source_role` | `official`、`professional`、`discovery` | 影响 LLM 相关性判断和来源标记 |
| `enabled` | `true`/`false` | 控制本次运行是否采集 |
| `rate_limit` | `250ms`、`2s`、`10/m`、`1/5s` 等 | 控制同一来源请求节奏 |

默认代码配置包含 OpenAI Blog、TechCrunch AI、Simon Willison、MIT Technology Review AI、Anthropic News、Google DeepMind、Cloudflare AI、NVIDIA Generative AI、人人都是产品经理、极客公园、Reddit LocalLLaMA，以及 AIHOT 公共 API。具体启停和 URL 以默认配置或 `AIHOT_SOURCES_FILE` 为准。

### RSS/Atom

实现：`server/internal/ingest/rss.go`。

- 使用 `gofeed` 解析 RSS 和 Atom。
- `GET` 请求，设置自有 User-Agent 和 RSS/Atom `Accept`。
- 提取标题、发布时间、描述/正文、feed 图片、enclosure 中的图片或视频。
- 默认单源超时 30s；重试次数默认 2，最多限制为 4。
- 重试状态包括 408、425、429 和 5xx；使用带上限的退避，默认退避基准 250ms，上限 3s。
- 单条没有链接的 feed item 会被跳过。
- 支持通过 `AIHOT_SOURCE_HTTP_PROXY` 设置采集侧代理。

### Anthropic News HTML

实现：`server/internal/ingest/html_anthropic.go`。

- `GET` 指定新闻页，使用 `goquery` 解析 DOM。
- 只接受同 host 的 `/news/` 链接，避免把导航、外链和无关页面当作资讯。
- 从链接节点或 title 节点提取标题。
- 尝试解析附近的 `<time>`，支持 RFC3339、`YYYY-MM-DD` 和英文日期格式。
- 有单次最大条数限制，不做递归全站爬取。

### 本地 Markdown corpus

实现：`server/internal/ingest/mpsource.go`。

- 通过 `AIHOT_MP_CORPUS_DIR` 指向只读 Markdown 文件树。
- 解析简化 frontmatter：`url`、`mp_name`、`title`、`publish_time`、`image_url`、`pic_url`。
- 没有明确图片时，回退到 Markdown 第一张图片。
- 先对标题和正文前 300 个 rune 做粗粒度 AI 关键词预筛，关键词覆盖 agent、LLM、RAG、MCP、Claude、GPT 等；不会把单独的 `ai` 作为唯一命中条件。
- 默认 `source_role=professional`；`AIHOT_MP_OFFICIAL_ACCOUNTS` 可以将指定公众号标成 `official`。
- 这是本地文件读取，不依赖海外网页 OG 请求，因此适合在采集网络受限时保留公众号素材。

### AIHOT v1 items API

实现：`server/internal/ingest/aihot.go`。

- 使用 HTTP `GET` 访问配置的 AIHOT endpoint。
- 默认追加 `mode=all`、`window=7d`、`by=published`、`limit=50`。
- 处理 `200` 和 `304`。
- 读取 JSON `items`，使用 `id`、`title`、`originalTitle`、`summary`、`source.name`、`links.aihot`、`links.original`、`publishedAt` 等字段。
- 优先使用原文链接；没有原文时使用 AIHOT 链接，并标记为 `AIHOT · 二手线索`。
- 对二手线索有单次数量上限，默认 `max_secondary=10`。
- 不继承 AIHOT 的 score、selected、reason，也不会因为 AIHOT item 自动访问原文页面。
- 使用独立的 `AIHOT_SOURCE_HTTP_PROXY`，不与 LLM 代理混用。

### 原文正文和媒体补全

实现：`server/internal/ingest/ogfetch.go`。

- 对 RSS/HTML 来源的原始链接做 best-effort `GET`。
- 默认超时 15s，响应体上限 3 MB。
- 通过正则读取 Open Graph 和 Twitter Card 的图片/视频。
- 使用 Mozilla Readability 提取正文 HTML。
- 只有正文长度达到最低阈值时才替换现有正文，并要求提取正文显著长于当前内容，避免短页面污染原始 feed 内容。
- 网络失败、页面结构不兼容或正文过短时，保留原始采集结果，不让补全失败阻断整条资讯。

### 采集层通用策略

一次 `pulse` 的处理顺序是：

1. 加载默认来源或 `-sources` 文件。
2. 对每个 enabled source 按来源串行采集。
3. 将来源错误记录到结果中，继续处理其他来源。
4. 规范化 URL，生成稳定 raw ID。
5. `raw_items` 幂等插入。
6. 优先处理 AIHOT raw，再按 `fetched_at` 处理普通 raw。
7. 对每条 raw 做正文/媒体补全、LLM 富化、翻译和术语抽取。
8. item 写入成功后标记 raw processed；单条失败则保留未处理状态，等待下一次重试。

当前 pulse 默认约束：

- `batchSize=20`
- 最多 `maxBatches=5`
- 单次最多约 100 个 enrichment 调用
- `maxItemAge=14d`
- 单条处理上下文约 3 分钟
- 没有进度时停止 drain，避免 poison item 无限空转

## URL 规范化、ID 和去重

`server/internal/ingest/rawitem.go` 对链接做 canonicalization：

- scheme 和 host 小写化
- 删除默认端口
- 删除 fragment
- 删除 `utm_*`、`fbclid`、`gclid` 等跟踪参数
- 清理 path
- 对 query 参数排序
- 非 URL 的外部标识保留为 seed

规范化后的 URL 通过 SHA-256 生成十六进制 raw ID，raw ID 继续作为后续 item 的稳定 ID 来源。

数据库层的去重和更新策略：

- `raw_items`：`INSERT ... ON CONFLICT(id) DO NOTHING`，已抓取原文不被静默覆盖。
- `items`：按 ID upsert，冲突时更新所有可变字段。
- 工具：按 GitHub node ID 以及 `(owner, repo)` 唯一约束去重。
- 聚类：每轮替换当前窗口结果，不在 item 表上重复生成同事件副本。

## PostgreSQL 和入库策略

### 连接与环境

| 环境 | 默认 DSN/数据库 | 用途 |
|---|---|---|
| 本地生产数据克隆 | `aihot_local_prod_clone` | 本地页面和自动任务联调 |
| 测试 | `aihot_test` | Go 数据库测试 |
| 部署 | `aihot` | Melos 环境生产数据 |

实际连接通过 `AIHOT_DATABASE_URL` 注入。工具发现使用 `AI_TOOL_DATABASE_URL`，未设置时部署脚本通常回退到 `AIHOT_DATABASE_URL`。

### 领域表

| 表 | 主要内容 | 写入策略 |
|---|---|---|
| `raw_items` | 来源原文、抓取时间、媒体、处理状态 | 稳定 ID 幂等插入；processed 成功后推进状态 |
| `items` | 标准化资讯、摘要、正文、译文、评分、选中状态、媒体 | ID upsert；详情页需要时更新译文和模型 |
| `item_terms` | item 与 entity/topic 术语关系 | 按 item 事务性 delete + insert，清理过期术语 |
| `dailies` | 日报日期、导语、sections、flashes | 按日期 upsert，sections/flashes 使用 JSONB |
| `clusters` | 当前窗口的主条目、来源数、来源名、热度 | 事务内 replace-all，保证整轮结果一致 |
| `tool_discovery_configs` | 工具发现配置和触发方式 | 软删除，状态和最近运行信息更新 |
| `tool_discovery_runs` | 每次发现运行、分页游标、暂停和失败信息 | 一次运行一条记录，支持继续和暂停 |
| `tool_runtime_settings` | GitHub base URL、Token 策略、分页和快照参数 | JSONB/结构化字段保存运行时设置 |
| `tools` | GitHub 工具主表、用途、状态、团队信息 | GitHub node ID 和 owner/repo 唯一 |
| `tool_discovery_sources` | 工具来自 keyword/topic/manual 等来源 | 记录来源和来源配置 |
| `tool_evaluations` | 评价状态、评价人、Cooper URL | 评价流程更新 |
| `tool_status_events` | 工具状态流转审计 | 追加事件 |
| `tool_member_reviews` | 团队成员评价 | 评价记录 |
| `tool_star_snapshots` | GitHub star 快照 | 按采集时间追加 |

Schema 位于各领域目录下并通过 `//go:embed schema.sql` 嵌入。`EnsureSchema` 使用 PostgreSQL 多语句和 `ALTER ... IF NOT EXISTS` 兼容增量字段，但它不是完整迁移框架。

### 查询、索引和分页

资讯查询特点：

- `items` 默认只返回 `present=true` 且 `duplicate_of_id IS NULL` 的可展示条目。
- selected 模式会折叠 cluster secondary，只展示主条目。
- 支持 `category`、`source_kind`、`since`、`until`、`q`、`score_min`。
- 使用 `(sort key, id)` 组合 keyset cursor，不依赖 offset 深分页。
- 列表查询主动将正文和原因列为 `NULL::text`，避免大字段 detoast；详情接口再读取完整正文。
- `q` 对标题、英文标题、摘要和正文使用 `ILIKE`。
- 数据库有 `pg_trgm` 以及标题、摘要、正文相关 GIN/排序索引。
- 公共搜索并发限制为最多 6 个，避免搜索把 PostgreSQL CPU 打满。

术语查询当前是实时 SQL，不做独立搜索引擎或预计算索引；当前规模下由 `item_terms` 和排除 duplicate item 的查询完成。

### 事务和失败恢复

- 工具状态、评价、来源、星标快照等多步写入尽量使用事务。
- 聚类替换是事务性的，避免只写入半轮结果。
- 日报按日期 upsert，支持重生成。
- raw item 处理成功后才标记 processed；LLM、正文补全或术语写入失败时保留重试机会。
- 术语抽取失败不会让已写入 item 失败，属于 best-effort 附属数据。

## LLM 调用

### 协议

`server/internal/llm/client.go` 调用内部 Anthropic-compatible HTTP proxy：

```text
POST {AIHOT_LLM_BASE_URL}/v1/messages
```

默认 base URL：

```text
https://llm-proxy.intra.xiaojukeji.com
```

请求头：

```text
x-api-key: {AIHOT_LLM_API_KEY}
anthropic-version: 2023-06-01
content-type: application/json
```

请求体结构：

```json
{
  "model": "deepseek-v4-flash",
  "max_tokens": 8192,
  "system": "...",
  "messages": [
    {"role": "user", "content": "..."}
  ]
}
```

只取响应 `content` 中第一个 `type=text` block。

### 模型和调用方

| 调用场景 | 固定模型 | 兼容环境变量（不改变模型） | 输出/用途 |
|---|---|---|---|
| 资讯结构化富化 | `deepseek-v4-flash` | `AIHOT_LLM_MODEL` | 中文标题、摘要、分类、相关性、评分、理由 |
| 普通正文翻译 | `deepseek-v4-flash` | `AIHOT_TRANSLATE_MODEL` | 保留 HTML、图片、链接的中文正文 |
| 详情页重新翻译 | `deepseek-v4-flash` | `AIHOT_TRANSLATE_RETRY_MODEL` | 用户触发的再次翻译 |
| 术语抽取 | `deepseek-v4-flash` | `AIHOT_TERMS_MODEL` | 最多 5 个实体、最多 3 个主题 |
| 日报导语 | `deepseek-v4-flash` 客户端 | `AIHOT_LLM_MODEL` | 日报标题和 2-4 句导语 |
| 热点同事件聚类 | `deepseek-v4-flash` 客户端 | `AIHOT_LLM_MODEL` | `item_ids` 分组 |
| 工具用途分类 | 工具发现侧传入的 LLM client | 通常使用 `AIHOT_TERMS_MODEL` | 从候选用途标签中选最多 3 个 |
| 工具中文摘要 | 工具 API summary generator | 由启动时创建的 LLM client 决定 | 短中文描述，失败时本地回退 |

### 资讯富化输出

`pipeline/enrichment.go` 要求模型输出单个 JSON 对象：

```json
{
  "title_cn": "...",
  "summary_cn": "...",
  "category": "ai-products",
  "relevance": 4,
  "score": 4,
  "reason_cn": "..."
}
```

处理规则：

- 允许从 code fence 或混杂文本中提取第一个 JSON 对象。
- 分类会做别名归一化，标准值为 `ai-models`、`ai-products`、`industry`、`paper`、`tip`。
- `relevance` 和 `score` 都限制在 1-5。
- selected 规则是 `relevance >= 4 && score >= 3`。
- `source_role` 会进入 prompt，区分 official、professional、discovery 来源。
- 富化输入正文会截断到约 4000 rune。

### 翻译和术语

- 翻译 prompt 要求只输出中文 HTML，保留标签、图片和链接。
- 翻译输入上限约 12000 rune。
- 如果输出像拒答、没有足够中文字符或破坏了结构，会被判为失败。
- RSS/HTML 正文支持翻译；Markdown corpus 已经是中文时通常不再整篇翻译。
- AIHOT 二手线索的摘要不会自动触发完整原文翻译。
- 术语抽取限制实体最多 5 个、主题最多 3 个，每个术语做长度、重复和类型清理。

### 日报和热点

- `gendaily` 没有 LLM key 时仍能生成日报，只是不生成导语。
- `hotpass` 要求 LLM key，因为同事件分组是它的核心职责；输出中的未知 ID、重复 ID、单条 cluster 都会使整轮失败且不写入。
- `gendaily` 默认重生成前一天和当天，按北京时间窗口组织日报；显式 `-date` 用于单日回填或重生成。

### 重试、超时和降级

- LLM HTTP client timeout 为 120s。
- 传输错误、429、5xx 最多尝试 5 次。
- 默认指数退避为 2s、4s、8s、16s，并尊重数值型 `Retry-After`。
- 401 和其他非 transient 错误不重试。
- 工具摘要有约 300ms 的生成时间预算，超时、结果不是中文或结果缺失时使用本地回退摘要，并记录 temporary summary。
- 日报导语、术语抽取和正文补全属于不同程度的 best-effort；主 item 已落库时，附属能力失败不会全部回滚。

需要特别区分：工具发现不使用 LLM 做 GitHub 仓库的最终“好坏评价”，但当前代码确实使用 LLM 做用途分类和中文摘要生成。

## 公共 API

### HTTP API 入口

标准入口在 `server/cmd/webserver/main.go` 注册以下路径：

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/healthz` | 数据库 ping 健康检查 |
| `GET` | `/ping` | 轻量存活检查 |
| `GET` | `/api/public/version` | 返回 API/Skill 版本 |
| `GET` | `/api/public/items` | 资讯列表 |
| `GET` | `/api/public/daily` | 最新日报 |
| `GET` | `/api/public/daily/{date}` | 指定日期日报 |
| `GET` | `/api/public/dailies` | 日报列表 |
| `GET` | `/api/public/hot-topics` | 热点主题 |
| `GET` | `/api/public/graph/cloud` | 词云数据 |
| `GET` | `/api/public/graph/term/{term}` | 指定术语关联图谱 |
| `GET` | `/items/{id}` | 资讯详情 SSR |
| `POST` | `/items/{id}/retranslate` | 详情页重新翻译，需启用 LLM |
| `GET/POST` | `/api/tools/*` | 工具发现和团队池 API |

### 资讯列表 API

`GET /api/public/items` 支持：

| 参数 | 说明 |
|---|---|
| `mode` | `selected` 或 `all`，默认 selected |
| `category` | 可重复传入的分类过滤 |
| `source_kind` | 可重复传入 `rss`、`html`、`api`、`mp`、`aihot` |
| `score_min` | 最低 score |
| `since` / `until` | 时间范围 |
| `q` | 关键词；至少 2 个字符，最多 200 个字符 |
| `take` | 1-100，配合 cursor |
| `cursor` | 不透明 keyset cursor |

默认浏览窗口为最近 7 天；有搜索词或浏览 MP 归档时可走全时间范围。响应包含 `count`、`hasNext`、`nextCursor` 和 `items`。

响应缓存：

- 使用弱 ETag，内容变化时返回新 ETag。
- `If-None-Match` 命中时返回 `304`。
- 缓存控制为 `public, s-maxage=300, stale-while-revalidate=300`。
- 未知或不合法参数返回 `400`，而不是静默忽略。

### 日报、热点和图谱 API

| API | 关键行为 |
|---|---|
| `/api/public/daily` | 获取最新有内容日报 |
| `/api/public/daily/{date}` | 日期格式为 `YYYY-MM-DD` |
| `/api/public/dailies` | `take` 严格限制 1-180，默认 30 |
| `/api/public/hot-topics` | 返回热点 cluster 和热度 |
| `/api/public/graph/cloud` | `window=7d|30d|all`，也支持北京日期参数 |
| `/api/public/graph/term/{term}` | 返回术语关联 item/term 数据，路径需要正确 URL 编码 |

热点和图谱同样使用 ETag 或窗口参数控制缓存，数据来源是 PostgreSQL 中的 cluster 和 item_terms。

### 工具 API

工具 API 统一前缀为 `/api/tools`，JSON 响应 envelope 为：

```json
{
  "errno": 0,
  "errmsg": "ok",
  "data": {}
}
```

错误按验证、冲突、限流、上游 GitHub/网络和内部错误映射到 400、409、429、502、500。

当前端点：

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/api/tools/settings` | 读取 GitHub 运行设置和 Token 脱敏信息 |
| `POST` | `/api/tools/settings` | 保存 GitHub base URL、Token 策略、分页和快照配置 |
| `POST` | `/api/tools/settings/tokens` | 新增 GitHub Token |
| `POST` | `/api/tools/settings/tokens/delete` | 删除 GitHub Token |
| `POST` | `/api/tools/settings/tokens/test` | 测试单个 Token |
| `POST` | `/api/tools/settings/check-github` | 检查整组 GitHub 配置 |
| `GET` | `/api/tools/configs` | 查询发现配置 |
| `POST` | `/api/tools/configs` | 新建或更新 keyword/topic 配置 |
| `POST` | `/api/tools/configs/enable` | 启用/停用配置 |
| `POST` | `/api/tools/configs/delete` | 删除配置 |
| `POST` | `/api/tools/configs/run` | 手动启动配置运行 |
| `POST` | `/api/tools/configs/pause` | 请求暂停运行 |
| `POST` | `/api/tools/configs/resume` | 继续运行 |
| `GET` | `/api/tools/items` | 查询 discovered/evaluating/included/excluded 工具 |
| `POST` | `/api/tools/items/update` | 更新工具元数据 |
| `POST` | `/api/tools/reviews/add` | 新增成员评价 |
| `POST` | `/api/tools/summaries/url-key` | 生成或获取工具中文摘要 |
| `POST` | `/api/tools/purpose-tags` | 更新用途标签，支持人工标签保护 |
| `POST` | `/api/tools/manual/preview` | 预览手动加入的 GitHub 仓库 |
| `POST` | `/api/tools/manual/add` | 手动加入仓库 |
| `POST` | `/api/tools/team/import` | 导入团队工具 |
| `POST` | `/api/tools/team/include` | 将发现工具加入团队池 |
| `POST` | `/api/tools/team/update` | 更新团队工具信息 |
| `POST` | `/api/tools/team/delete` | 从团队池删除 |
| `POST` | `/api/tools/discovered/exclude` | 排除发现工具 |
| `POST` | `/api/tools/evaluations/start` | 开始评价 |
| `POST` | `/api/tools/evaluations/cooper-url` | 保存 Cooper 评价链接 |
| `POST` | `/api/tools/evaluations/finish` | 完成评价并更新状态 |

工具写接口当前由 handler 直接提供，本文未发现独立认证/RBAC 中间件；如果要将这些 POST 接口暴露到不可信网络，需要在部署层或服务层补充访问控制。

## GitHub 工具发现协议

### GitHub REST

实现：`server/internal/tools/github.go`、`server/internal/tools/discovery.go`。

- 默认 base URL：`https://api.github.com`，可配置。
- `GET /search/repositories`
- `GET /repos/{owner}/{repo}`
- `GET /rate_limit`
- 请求头：`Accept: application/vnd.github+json`、`X-GitHub-Api-Version: 2022-11-28`、`User-Agent: ai-tool/0.1`。
- 有 Token 时使用 `Authorization: Bearer ...`。
- 单请求超时 30s。
- 识别 429、403 和 rate-limit headers。

默认搜索策略：

```text
"term" in:name,description,topics,readme fork:false archived:false
topic:term fork:false archived:false
```

如果已有上次成功运行时间，会追加 `created`/`pushed` 的增量时间条件，并向前扩展约 24 小时，降低边界时间丢数据的概率。

### Token 池和限流

支持：

- `AI_TOOL_GITHUB_TOKEN`
- `AI_TOOL_GITHUB_TOKENS`
- 数据库运行时设置中的 Token 列表

Token 策略：

- `round_robin`
- `fixed`
- `failover`

失效 Token 会被跳过；在允许时可以回退匿名请求。每页请求之间支持间隔，默认约 2s；默认最多 5 页，每页默认 100 条，星标快照默认上限 200。

### 发现运行状态

发现任务持久化 query index、page、成功时间、扫描页数、结果数量、暂停请求和失败原因，因此可以：

- 手动运行
- 定时运行
- 暂停后继续
- 标记 partial/incomplete/truncated
- 记录来源是 keyword、topic 还是 manual
- 追加 GitHub star snapshot

发现调度使用 `Asia/Shanghai`。daily/weekly 配置由每分钟触发器检查，`LastRunAt` 防止同一个时间槽重复启动；保留 `weekly` 命令作为旧入口兼容。

## 调度、部署和运行时

### 任务进程

| 二进制/脚本 | 作用 |
|---|---|
| `server` | Go HTTP 服务 |
| `pulse` | 采集、raw 入库、LLM 富化和 item 入库 |
| `gendaily` | 生成日报 |
| `hotpass` | LLM 同事件聚类和热度 |
| `discovertools` | 工具发现、快照、调度和手动运行 |
| `importmp` | 导入 Markdown corpus |
| `deploy/aihot-cron.sh` | 统一环境变量、锁、日志和命令执行 wrapper |

### 部署拓扑

```text
用户浏览器
    │
    ▼
Nginx :8899
    ├── /var/www/aihot      → web/dist 静态资源
    ├── /api/*              → 127.0.0.1:8991
    ├── /items/*            → 127.0.0.1:8991
    └── /healthz            → 127.0.0.1:8991
                                  │
                                  ▼
                         supervisor 常驻 Go server
                                  │
                                  ▼
                         PostgreSQL 数据库 aihot
```

本地开发通常是 Vite 前端和 Go 后端分开运行；AI Cool 本地联调约定使用 `http://127.0.0.1:5178`，后端使用 `:8991`，数据使用 `aihot_local_prod_clone`。Vite/Playwright 的代码默认端口仍分别是 `5173`/`5176`，因此自动化和本地启动脚本可能会覆盖默认值。

### 当前调度类型

部署脚本和本地 runbook 支持：

- `pulse`：周期性采集和处理
- `gendaily`：生成前一天/当天日报
- `hotpass`：热点聚类
- `discovertools scheduled`：每分钟唤醒并检查 daily/weekly 配置
- snapshot：按周保存工具星标快照

`deploy/aihot-cron.sh` 对 `pulse` 和 `discovertools` 使用 `flock` 锁，避免上一次还未结束时并发启动；日志写入 `/root/aihot/log/*.log`。

### 关键配置

#### 资讯、数据库和 HTTP

| 环境变量 | 作用 |
|---|---|
| `AIHOT_DATABASE_URL` | 资讯/日报/热点/工具主数据库 DSN |
| `AI_TOOL_DATABASE_URL` | 工具发现数据库 DSN，部署时可回退到 `AIHOT_DATABASE_URL` |
| `AIHOT_HTTP_ADDR` | Go HTTP 监听地址 |
| `AIHOT_SOURCE_HTTP_PROXY` | 资讯源 HTTP 代理 |
| `AIHOT_SOURCES_FILE` | 外部资讯来源 JSON |
| `AIHOT_MP_CORPUS_DIR` | Markdown corpus 路径 |
| `AIHOT_MP_OFFICIAL_ACCOUNTS` | 公众号 official 覆盖配置 |
| `AI_TOOL_WEB_API_PROXY` | Vite `/api` 和 `/items` 代理目标 |

#### LLM

| 环境变量 | 作用 |
|---|---|
| `AIHOT_LLM_API_KEY` | LLM 必填密钥 |
| `AIHOT_LLM_BASE_URL` | Anthropic-compatible proxy 地址 |
| `AIHOT_LLM_MODEL` | 历史兼容变量；运行时固定使用 `deepseek-v4-flash` |
| `AIHOT_TRANSLATE_MODEL` | 历史兼容变量；运行时固定使用 `deepseek-v4-flash` |
| `AIHOT_TRANSLATE_RETRY_MODEL` | 历史兼容变量；运行时固定使用 `deepseek-v4-flash` |
| `AIHOT_TERMS_MODEL` | 历史兼容变量；运行时固定使用 `deepseek-v4-flash` |

#### GitHub 工具发现

| 环境变量 | 作用 |
|---|---|
| `AI_TOOL_GITHUB_TOKEN` | 单个默认 GitHub Token |
| `AI_TOOL_GITHUB_TOKENS` | 多 Token 配置 |
| `AI_TOOL_GITHUB_BASE_URL` | GitHub REST base URL |
| `AI_TOOL_GITHUB_MAX_PAGES` | 最大搜索页数 |
| `AI_TOOL_GITHUB_PER_PAGE` | 每页数量 |
| `AI_TOOL_GITHUB_REQUEST_INTERVAL_MS` | 请求间隔 |
| `AI_TOOL_STAR_SNAPSHOT_LIMIT` | 星标快照数量上限 |

当前代码实际使用的是 `AI_TOOL_*` 和 `AIHOT_*` 命名；不要把 `AICOOL_GITHUB_TOKEN`、`AICOOL_GITHUB_MAX_PAGES` 等产品名变量当成已实现的环境变量，除非部署脚本另行做了映射。

## 可用性、性能和安全边界

### 可用性

- 一个资讯源失败不会阻断其他来源。
- 单条 raw 处理失败会留在未处理队列中。
- LLM 对 429/5xx/传输错误重试。
- 正文补全、译文和术语是分级 best-effort。
- 工具发现支持 partial、pause、resume 和 failover。
- cron wrapper 使用锁，避免同一任务重入。

### 性能

- pulse 有批量数和总 enrichment 上限。
- 列表使用 keyset pagination。
- 列表避免读取大正文，详情单独读取。
- 搜索并发限制为 6。
- public API 使用 ETag 和共享缓存控制。
- 工具 GitHub 请求有 page 上限、请求间隔和 Token 池。

### 安全和权限

- LLM key、GitHub Token 通过环境变量或数据库脱敏展示，不应写入前端日志和普通响应。
- GitHub Token API 返回 masked/last4，而不是原始 Token。
- 当前公共 handler 代码没有清晰的登录、鉴权或 RBAC 层；工具写 API 的外部暴露边界依赖 Nginx、网络隔离或部署侧访问控制。
- `AIHOT_SOURCE_HTTP_PROXY` 和 LLM proxy 是两条独立出口链路，避免将资讯源代理误用于内部模型请求。
- 详情翻译和工具摘要会把外部内容送入 LLM，应继续关注 prompt injection、HTML 清理和敏感信息泄露。
- Markdown、RSS 和网页正文最终进入 HTML 展示，`bluemonday`、正文提取和前端渲染边界需要持续回归 XSS。

### 健康检查边界

`/healthz` 主要验证数据库 ping，不等同于：

- 所有表已经成功创建
- 资讯采集任务正常
- LLM key 可用
- GitHub Token 有额度
- 前端能拿到业务数据

部分服务启动阶段的 `EnsureSchema` 是 best-effort；服务可能先启动，直到访问对应业务路由时才暴露 schema 错误。因此排障需要分层检查：进程、`/healthz`、真实业务 API、数据库表和最近任务日志。

## 测试、构建和验证

### 后端

```bash
cd server
make test
make build
```

`make test` 当前通过 `go test -p 1 ./...` 串行执行，因为多个数据库测试包共享表并会 truncate 数据。

### 前端

```bash
cd web
npm run build
npm run lint
npx vitest run
```

浏览器回归：

```bash
cd web
PLAYWRIGHT_BASE_URL=http://127.0.0.1:5178 npx playwright test e2e/ai-tool-cases.spec.ts
```

运行 Playwright 前应确认后端 API、Vite 代理、数据库和目标页面都是真实可用的；SPA fallback 返回 200 只能证明前端壳存在，不能证明数据链路或数据库链路正常。

## 需要持续核对的边界

以下内容不能仅凭“能启动”判定完成：

1. `server` 是否使用标准 `net/http` 入口还是 Nuwa 入口，需以实际部署进程和启动脚本确认。
2. `server/conf/app.toml` 中的 Redis/MySQL 配置属于兼容基础设施还是仍被某条业务链路使用，需结合部署环境再次确认。
3. 生产环境的 cron 内容可能与仓库脚本、最近安装结果存在时间差，应以服务器上的 crontab、supervisor 状态和任务日志为准。
4. `AIHOT_LLM_API_KEY` 存在不代表模型调用成功，还需要验证 `/v1/messages` 的真实状态、重试和额度。
5. `/healthz`、`/ping` 或静态页面 200 都不是资讯入库、LLM 富化或工具发现成功的证据。
6. 工具评价数据与 Cooper 的关系是保存/跳转 URL；当前仓库没有把 Cooper 文档内容当作本地评价事实自动同步。

## 常用源码定位

| 主题 | 路径 |
|---|---|
| 前端入口和 hash 路由 | `web/src/App.tsx` |
| 前端代理、Vitest | `web/vite.config.ts` |
| Playwright | `web/playwright.config.ts` |
| Go 依赖 | `server/go.mod` |
| 标准 HTTP 服务 | `server/cmd/webserver/main.go` |
| Nuwa 服务 | `server/common/server/httpserv/httpserv.go` |
| 资讯来源配置 | `server/internal/pulse/sources.go` |
| RSS/Atom | `server/internal/ingest/rss.go` |
| Anthropic News HTML | `server/internal/ingest/html_anthropic.go` |
| Markdown corpus | `server/internal/ingest/mpsource.go` |
| AIHOT API | `server/internal/ingest/aihot.go` |
| 正文/OG 补全 | `server/internal/ingest/ogfetch.go` |
| raw 入库 | `server/internal/ingest/store.go` |
| LLM client | `server/internal/llm/client.go` |
| 资讯富化 | `server/internal/pipeline/enrichment.go` |
| 翻译 | `server/internal/pipeline/translate.go` |
| 术语 | `server/internal/pipeline/terms.go` |
| 日报 | `server/internal/daily` |
| 热点聚类 | `server/internal/cluster`、`server/cmd/hotpass` |
| 公共 API | `server/internal/publicapi` |
| 工具发现 | `server/internal/tools`、`server/cmd/discovertools` |
| 部署 wrapper | `deploy/aihot-cron.sh` |
| Nginx | `deploy/aihot.nginx.conf` |
| 本地自动运行 | `docs/runbooks/local-auto-run.md` |
