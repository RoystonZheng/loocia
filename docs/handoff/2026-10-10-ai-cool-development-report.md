# AI Cool 开发报告
## 1. 信息源扩展

### 1.1 PRD 来源清单与完成情况
本期按 [`ai-cool-source-expansion-prd.md`](../design/ai-cool-source-expansion-prd.md) 的来源清单实现。采集方式写入 `source_kind`，来源使用口径写入 `source_role`，两者不再混用。
| PRD 信息源 | 接入方式 | 当前状态 | 还需要做什么 |
|---|---|---|---|
| OpenAI Blog | RSS/Atom，复用 `RSSSource` | 已接入，默认启用 |  |
| TechCrunch AI | RSS/Atom，复用 `RSSSource` | 已接入，默认启用 |  |
| Simon Willison | Atom，复用 `RSSSource` | 已接入，默认启用 |  |
| MIT Technology Review AI | RSS，复用 `RSSSource` | 已接入，默认启用 |  |
| Anthropic News |    新增 HTML adapter，只解析同域 `/news/` 链接 | 代码和默认配置已完成 | 生产机需要再做一次真实页面 smoke，防止页面结构变化后静默失效 |
| Google DeepMind | RSS/Atom，复用 `RSSSource` | 已接入，默认启用 |  |
| Cloudflare AI | RSS，复用 `RSSSource` | 已接入，默认启用 |  |
| NVIDIA Generative AI | RSS，复用 `RSSSource` | 已接入，默认启用 |  |
| 人人都是产品经理 | RSS，复用 `RSSSource` | 已接入，默认启用 |  |
| 极客公园 | RSS，复用 `RSSSource` | 已接入，默认启用 |  |
| Reddit LocalLLaMA | RSS/Atom，设置超时和重试 | 已接入，默认配置启用 | 生产机需要确认限流情况 |
| Reddit MachineLearning | RSS/Atom | 已接入代码，但默认关闭；本地验证曾返回 429 | 部署机确认限流后再启用 |
| 已有公众号语料 | 读取 `AIHOT_MP_CORPUS_DIR` 下的 Markdown，不做微信直连 | adapter 已完成；配置目录存在时才启用 | 部署时配置语料目录；需要官方账号覆盖时配置 `AIHOT_MP_OFFICIAL_ACCOUNTS` |
| AIHOT | 使用公开 `https://aihot.news/api/v1/items` | 已接入，默认启用，作为 `discovery` 补漏源 | 生产机确认 API 频率和授权边界 |


### 1.2 各类来源如何接入
#### RSS/Atom
`server/internal/ingest/rss.go` 继续使用 `gofeed`。`server/internal/pulse/sources.go` 将配置转换为 `RSSSource`，透传以下参数：
- `source_kind=rss`
- `source_role=official|professional|discovery`
- 单轮抓取上限 `max_items_per_run`
- 超时、重试次数和 `rate_limit`
没有链接的条目会跳过；发布时间、正文摘要、图片和视频沿用现有 RSS 解析逻辑。

#### Anthropic News HTML
`server/internal/ingest/html_anthropic.go` 新增 `AnthropicNewsSource`：
1. 请求 `https://www.anthropic.com/news`。
2. 用 `goquery` 解析页面。
3. 只接受同一 host、路径以 `/news/` 开头的链接。
4. 从链接文本和标题节点取标题，从相邻 `<time>` 取日期。
5. 生成 `source_kind=html`、`source_role=official` 的 `RawItem`。
解析不到新闻链接时本源报错，由 `Runner` 记录 source error，其他来源继续运行。

#### 公众号语料
`server/internal/ingest/mpsource.go` 读取 `AIHOT_MP_CORPUS_DIR` 下的 Markdown 文件，解析 frontmatter 中的 `url`、`mp_name`、`title`、`publish_time`、`image_url/pic_url`。标题和正文前 300 个字符先做 AI 关键词粗筛，命中后才进入 raw 队列。
默认角色是 `professional`；`mp_name` 命中 `AIHOT_MP_OFFICIAL_ACCOUNTS` 时改成 `official`。这条链路只读本地语料，不新增微信接口。

#### AIHOT API v1
`server/internal/ingest/aihot.go` 调用公开 items 接口，默认补齐 `mode=all`、`window=7d`、`by=published`、`limit=50`。条目有 `links.original` 时使用原文 URL；没有原文时只保留配置上限内的少量二手线索，并展示为 `AIHOT · 二手线索`。
AIHOT 的 `score`、`selected`、`reason` 不进入 AI Cool 的结论。AIHOT 只提供候选标题、摘要、来源和链接，后面仍走 AI Cool 自己的富化和精选规则；也不通过 PageResolver 追抓 AIHOT 页面正文。

#### 配置和兼容
`SourceConfig` 新增 `adapter`、`source_kind`、`source_role`、`enabled`、抓取上限、超时、重试、窗口和限流字段。旧的 `{name,url}` JSON 仍按 RSS + `discovery` 处理；`enabled=false` 的来源跳过。部署可以用 `AIHOT_SOURCES_FILE` 覆盖内置来源，不需要重新编译。

### 1.3 精选、去重和队列策略的变化
| 规则 | 原来 | 现在 | 代码位置 |
|---|---|---|---|
| 精选门槛 | `score >= 3` | `relevance >= 4 && score >= 3`，同时要求内容确实与 AI 相关且有阅读价值 | `server/internal/pipeline/processor.go` 的 `toItem` |
| 来源判断 | 只有来源名称，LLM 不区分来源角色 | `official`、`professional`、`discovery` 写入 raw，并注入 enrichment prompt；官方身份不自动加分，补漏和二手内容需要降权判断 | `rawitem.go`、`enrichment.go`、`store.go` |
| URL 去重 | 直接使用原 URL 计算 `RawID` | URL 先规范化：host/scheme 小写、去默认端口、fragment 和常见追踪参数，排序 query，清理 `.`/`..` 路径后再计算 SHA-256 | `server/internal/ingest/rawitem.go` 的 `CanonicalURL`/`RawID` |
| AIHOT 去重 |  无| 有原文时按规范化原文 URL 去重；没有原文时使用 `aihot:<id>`，并受二手线索上限限制 | `server/internal/ingest/aihot.go` |
| 未处理队列 | 按 `fetched_at` 老数据优先 | 先处理近 7 天数据，再处理 AIHOT，最后按 `fetched_at` 先进先出，避免历史 backlog 挡住新内容 | `server/internal/ingest/store.go` 的 `ListUnprocessed` |
| 模型输出解析 | 依赖模型一次返回合法格式 | category 别名归一，只取第一个合法 JSON 对象，相关度和 score 限制在 1-5 | `server/internal/pipeline/enrichment.go` |
本期仍按 PRD 只做规范化 URL 级去重，不做跨 URL 的语义去重，也不回刷历史 `items.selected`。这两点是设计边界，不是遗漏。


## 2. 工具发现改动

### 2.1 满足开发方案的部分
| 开发方案要求 | 已做改动 | 实现位置 |
|---|---|---|
| 独立工具域和数据模型 | 新增工具主表、发现配置、运行记录、运行设置、来源、测评、状态事件、团队评价、Stars 快照等表 | `server/internal/tools/schema.sql`、`types.go`、`store.go` |
| GitHub 关键词发现 | 调用官方 Repository Search API，支持查询词、分页、最大页数、`incomplete_results` 和查询索引记录 | `github.go`、`discovery.go` |
| GitHub Topic 发现 | 自动拼接 `topic:<topic>`，与关键词发现共用入库流程 | `discovery.go` |
| 手动添加仓库 | 校验 GitHub URL，调用 Repository API 获取仓库事实字段后入库 | `tools_handler.go` 的 manual preview/add |
| 去重和来源合并 | 使用 GitHub `node_id` 做唯一身份；再次命中只更新仓库事实、来源和最近命中时间，不重建工具 | `store.go`、`schema.sql` |
| 工具状态 | `discovered -> evaluating -> included/excluded`，状态变更带当前状态条件并写审计事件 | `store.go`、`status_test.go` |
| 测评记录 | 保存测评人、操作人、Cooper URL、开始/完成时间、纳入说明或不纳入原因；Cooper 文档仍由用户自己维护 | `tool_evaluations`、`tools_handler.go` |
| 团队工具 | 支持直接纳入、测评后纳入、手动导入、编辑和软删除 | `tools_handler.go`、`ToolsView.tsx` |
| 用途分类 | 自动分类优先使用 LLM，失败时本地规则兜底；支持新增标签和人工修正，人工修正不会被自动分类覆盖 | `purpose.go`、`store.go` |
| 多 Token 和运行参数 | 支持 Token 名称/描述/测试链接、脱敏预览、去重、轮询/固定/故障转移策略、Base URL、分页、间隔和 Stars 快照上限 | `tokens.go`、`github.go`、`tools_handler.go` |
| 调度和断点 | 支持 daily/weekly、北京时间 `HH:MM`、星期、每分钟 `scheduled` 检查、暂停/继续和运行检查点；`weekly` 保留兼容 | `schedule.go`、`cmd/discovertools/main.go` |
| 页面和接口 | 接入两个 HTTP 入口，提供配置、工具列表、测评、团队工具、评价、Token、批量操作等 `/api/tools/*` 接口 | `server/cmd/webserver/main.go`、`server/common/server/httpserv/httpserv.go`、`tools_handler.go` |
| 前端四个页面 | 配置、已发现、测评中、团队工具四个 hash 页面，支持来源/关键词/用途筛选、排序、分页、跨页批量和错误提示 | `web/src/components/ToolsView.tsx`、`web/src/api/tools.ts` |

### 2.2 具体交互和实现
工具发现链路现在是：
```text
创建关键词/Topic 配置或手动粘贴仓库
  -> GitHub 官方 API
  -> node_id 去重并合并发现来源
  -> discovered
  -> 批量开始测评并关联 Cooper URL
  -> evaluating
  -> 纳入团队工具或不纳入
  -> included / excluded
```
发现任务对首次运行做有限页数扫描；后续使用 `created`/`pushed` 增量窗口并保留 24 小时重叠。运行记录保存页码、查询索引、限流、截断和错误类别，失败不会删除已有工具。


### 2.3 当前状态和剩余工作
代码、接口、前端主流程和本地运行链路已经完成；本地真实验收结果见第 3 节。仍需要补的内容集中在运行环境，不是页面和状态机缺功能：
- 在正式库按 schema 建表，确认 Melos 上 `discovertools scheduled` 每分钟唤醒并在真实时间槽执行。
- 配置稳定的 GitHub Token，重跑一次关键词、Topic 和手动添加的正式闭环；最近一次 weekly 仍有 partial 记录。
- 准备独立测试库，补跑事务、并发、旧 schema、故障注入等数据库用例。


## 3. 测试

### 3.1 测试目标和证据口径
测试结果按证据层次记录：纯逻辑和组件测试、mock 接口测试、本地真实环境、外部服务和生产环境。`BLOCK` 表示当前条件不足以安全复现，不把它当成失败，也不把它算作通过。

### 3.2 测试用例生成
用例参考来源：
- 信息源和工具发现 PRD：功能范围、状态流转、来源清单和验收条件。
- 技术方案和 API 文档：表结构、字段校验、错误码、分页、Token、调度和部署约束。
- 当前代码：真实路由、Store 事务、前端交互、脚本和环境变量。
- 风险补充：重复命中、URL 变化、分页截断、GitHub 限流、LLM 空响应、暂停恢复、并发状态操作、错误 DSN 和生产/本地隔离。
先按功能流程、参数边界、数据一致性和组合条件拆测试点，再按执行条件分到 Go、Vitest、Playwright、本地真实验收此前测试分析得到 136 个测试点，随后扩展为 339 条细粒度 Spaceway 用例，覆盖正常、异常、边界、幂等、并发、调度、外部依赖和环境隔离。


### 3.3 测试集合

本次测试用例不是按代码文件分组，而是按 XMind 根节点“AI Cool 全量验收测试用例”下面的二级分类组织。共 11 个测试集合、339 条用例。每个集合下面还包含更细的三级分类和具体用例。

#### 全部测试用例和执行载体
- [339 条 XMind 测试集合](../../artifacts/spaceway-xmind/ai-cool-spaceway-xmind-339.xmind)
- [339 条用例 ID 清单](../../artifacts/spaceway-import/ai-cool-case-ids.txt)
| 测试集合 | 用例数 | 主要覆盖 |
|---|---:|---|
| 信息源采集 | 42 | 来源配置加载、RSS/Atom、Anthropic HTML、公众号语料、AIHOT、限流/重试、单源错误隔离、来源角色、URL 去重和原始数据入队 |
| 资讯自动采集 | 29 | `pulse` 主链路、raw 队列、采集与原始入库、`ingest-only`、单条失败重试、超时、断点和下游数据连续性 |
| 热点资讯评分与精选 | 26 | AIHOT 富化、LLM 输出解析、category 归一、相关度与 score、`relevance >= 4 && score >= 3` 双门槛、精选结果和历史数据不回刷 |
| 词云与关联图谱 | 15 | terms 提取、词云数据、图谱节点和关联关系、空结果、重复词、下游查询及异常处理 |
| 全部资讯筛选与 Feed | 36 | 全部/精选/归档 Feed、来源和主题筛选、搜索、分页 cursor、统计、ETag、空态/错误态和窄屏页面 |
| 工具操作与必填校验 | 75 | 工具发现、手动添加、测评、Cooper URL、纳入/不纳入、团队工具、评价、批量操作、状态冲突和必填字段 |
| 发现配置与自动运行 | 44 | 关键词/Topic 配置、首次和增量 GitHub 查询、24 小时重叠、分页、限流、Stars 快照、daily/weekly、北京时间、暂停/继续和运行记录 |
| 配置设置与参数校验 | 39 | 信息源、工具配置、调度时间、GitHub Token、Base URL、运行参数、数值范围、枚举、长度和非法输入 |
| 本地生产与上线风险 | 27 | 部署入口、cron、生产/本地 DSN 隔离、双 HTTP 入口、schema 迁移、事务、运行锁、日志脱敏和前端构建运行 |
| 跨场景组合与异常恢复 | 2 | AIHOT 与工具发现跨场景组合，以及手动 GitHub 入口和异常恢复的交叉场景 |
| 其他场景 | 4 | 资讯历史兼容、AIHOT 来源边界、GitHub 手动入口和运行结果的补充验收 |

### 3.4 执行命令和结果

本节以 2026-10-14 的最新本地复测为准。测试环境分为独立测试库、本地生产副本、外部服务 smoke 和生产只读检查；不同环境的结果不混写。

| 测试范围 | 执行命令或入口 | 最新结果 |
|---|---|---|
| 后端单测和数据库用例 | `cd server && AIHOT_TEST_DATABASE_URL=<aihot_test DSN> AI_TOOL_TEST_DATABASE_URL=<aihot_test DSN> GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test` | 退出码 0。后端各包通过，数据库用例已在本地 `aihot_test` 执行。两个 DSN 当前指向同一个本地测试库，因此“两个独立测试库”这一项还没有完成。 |
| Go 静态检查和后端构建 | `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go vet ./...`；`make build` | 均通过。 |
| 前端单测、构建和 lint | `cd web && npx vitest run`；`npm run build`；`npm run lint` | Vitest 19 个文件、114 条用例通过；构建和 lint 通过。仍有 React `act(...)` 警告，但没有测试失败。 |
| Playwright 页面、工具操作和数据库迁移 | `AICOOL_LIVE_SOURCE_TESTS=1 AIHOT_TEST_DATABASE_URL=<aihot_test DSN> npx playwright test e2e/source-expansion-cases.spec.ts e2e/ai-tool-cases.spec.ts` | 最新一次共 11 条测试：10 条通过，1 条失败。失败项是 `TC-E2E-003` 的 Anthropic 外部源 HEAD 请求，错误为 TLS 建连前被网络连接断开；`TC-E2E-004` 真实 PostgreSQL schema 迁移通过，工具页面和信息源页面测试通过。 |
| 本地真实链路 | `cd web && npm run test:e2e:local` | `ACCEPT-001`～`ACCEPT-008`，8/8 通过。链路使用 `127.0.0.1:5178`、`127.0.0.1:8991` 和 `aihot_local_prod_clone`。 |
| 本地真实信息源采集 | `AIHOT_SOURCE_HTTP_PROXY=<proxy> AIHOT_PULSE_INGEST_ONLY=1 ./server/scripts/local-aihot-cron-safe.sh pulse` | 进程退出码 0，结果为 `fetched=1465 inserted=3 skipped=193 ageskip=1269 srcerrs=4`。Google DeepMind、Cloudflare AI、NVIDIA 和人人都是产品经理仍有采集错误，不能记为全量来源通过。 |
| 本地调度检查 | `AI_TOOL_GITHUB_MAX_PAGES=1 AI_TOOL_GITHUB_REQUEST_INTERVAL_MS=0 AI_TOOL_STAR_SNAPSHOT_LIMIT=1 ./server/scripts/local-aihot-cron-safe.sh discovertools scheduled -actor local-smoke` | `checked=1 skipped=1 attempted=0`。调度检查入口正常，但本次配置未到执行时间，没有形成真实 GitHub 发现结果。 |
| GitHub 快照 | `AI_TOOL_STAR_SNAPSHOT_LIMIT=1 ./server/scripts/local-aihot-cron-safe.sh discovertools snapshot` | `attempted=1 saved=0 failed=1`。该次执行还没有使用新提供的 GitHub Token，需要配置 Token 后重跑。 |
| 真实 LLM smoke | `go test ./internal/llm ./internal/pipeline -run 'TestCompleteRealProxy\|TestProcessBatchRealLLM' -count=1 -v` | 充值前的旧记录：一条连续重试后返回 `EOF`，一条返回 `402 Insufficient Balance`。LLM 充值后尚未重跑，当前不能把这两条算作最终失败。 |
| 生产只读检查 | Melos SSH、线上 `/healthz`、生产服务和 crontab 检查 | SSH 返回 `Permission denied (publickey)`，线上 health check 返回空响应，生产 schema、Cron 和真实时间槽尚未形成证据。 |

339 条用例的 Spaceway 全量回写仍以 2026-10-08 的记录为准：`PASS 274`、`BLOCK 65`、写入后读回 `mismatches=[]`。`BLOCK` 主要来自独立数据库、并发和故障注入、真实 LLM/GitHub、生产调度和词云实体边界；它表示当前条件不足，不能直接等同于产品失败。

### 3.5 测试集合对应的模块状态

| 测试集合 | 用例数 | 当前状态 | 已完成内容 | 还差什么 |
|---|---:|---|---|---|
| 信息源采集 | 42 | 部分完成 | 来源配置、RSS/Atom、HTML adapter、公众号语料、AIHOT、URL 去重和错误隔离的代码测试通过；浏览器 live smoke 曾成功执行 | Go 采集器仍有 4 个来源错误；Anthropic 外部源最新回归受网络连接影响；需要稳定代理后重跑，并在生产机器确认来源可达 |
| 资讯自动采集 | 29 | 部分完成 | `pulse`、raw 队列、`ingest-only`、失败隔离和本地链路通过 | LLM 充值后重新执行真实富化；生产机补一次真实采集并检查 `srcerrs` |
| 热点资讯评分与精选 | 26 | 部分完成 | 精选门槛、JSON 解析、category 归一、分数边界和 Store 测试通过 | 真实 LLM 返回、日报和热点任务需要重跑；`hotpass` 的历史间歇退出码 1 需要确认 |
| 词云与关联图谱 | 15 | 部分完成 | terms、词云和图谱接口的代码测试通过 | 词云实体边界专项和真实 LLM 生成结果还没有独立运行证据 |
| 全部资讯筛选与 Feed | 36 | 已完成 | Vitest、Playwright 来源筛选、主题多选、分页、空态、错误态和窄屏用例通过 | 仅需在生产发布后做一次页面和接口 smoke |
| 工具操作与必填校验 | 75 | 已完成本地验证 | 工具发现页面、测评、Cooper URL 必填、纳入/不纳入、团队工具、批量操作和错误态通过 | 真实 Cooper/SSO/RBAC 权限边界仍未验证 |
| 发现配置与自动运行 | 44 | 部分完成 | 配置、暂停/继续、北京时间判断、运行记录和本地调度入口通过 | 新 GitHub Token 下重跑关键词、Topic、手动添加和 Stars 快照；生产 Cron 和 daily/weekly 时间槽还未验证 |
| 配置设置与参数校验 | 39 | 部分完成 | 信息源、Token、Base URL、调度和数值边界的代码及前端测试通过 | 新 Token 的真实连通性和生产配置读取需要验证 |
| 本地生产与上线风险 | 27 | 本地完成，生产未完成 | 本地 5178 → 8991 → `aihot_local_prod_clone`、构建、运行锁和本地 DSN 防误写通过 | 生产 schema、服务状态、Cron、日志和回滚检查还未执行 |
| 跨场景组合与异常恢复 | 2 | 部分完成 | 339 条用例已导入并完成一次全量回写 | 65 条 BLOCK 需要补环境后重跑，尤其是并发、故障注入和跨模块恢复 |
| 其他场景 | 4 | 部分完成 | 兼容和边界用例已纳入用例集 | AIHOT 来源边界、GitHub 手动入口和真实运行结果还需补证据 |

### 3.6 测试用例和执行附件

- [339 条 XMind 测试集合](../../artifacts/spaceway-xmind/ai-cool-spaceway-xmind-339.xmind)
- [339 条用例详细文档](../../artifacts/ai-cool-339-test-cases-cooper.md)
- [339 条用例 ID 清单](../../artifacts/spaceway-import/ai-cool-case-ids.txt)


### 3.7 尚未测试的内容和所需条件
| 未测试内容 | 需要提供或处理的条件 |
|---|---|
| 两套独立数据库上的事务、并发、迁移、回滚和故障注入 | 提供两个不指向生产库的 DSN，或提供隔离数据库和建库权限；当前 `aihot_test` 只有一个库，不能替代两套独立环境 |
 |
|  |
| 外部信息源真实采集 | 提供稳定的 HTTP/HTTPS 代理或网络白名单；在测试机和生产机分别记录来源状态、`srcerrs`、超时和限流结果 |
| 生产 schema、服务和 Cron | 提供 Melos SSH 或委托有权限的同事执行只读检查；需要确认 schema、服务版本、crontab、运行表和日志 |
|  |
| 339 条 BLOCK 的全量复测 | 上述环境具备后，按 XMind 二级集合逐组执行；每条记录实际结果、证据链接和 PASS/FAIL/BLOCK，不把设计预期直接改成通过 |

## 4. 已发现问题和处理计划
| 问题 | 已做改动 | 后续处理 |
|---|---|---|
| 新资讯被历史 raw backlog 挡住 | 队列改为近 7 天优先，再按 AIHOT 和 FIFO 排序 | 在独立测试库补排序、并发和断点验收 |
| Go 采集器在部分外部源上出现 EOF/TLS 连接错误 | 已支持 `AIHOT_SOURCE_HTTP_PROXY`，单源失败不会阻断整轮 | 固定测试/生产代理，逐源重跑；若仍失败，按源单独处理或暂时关闭问题源 |
| 无 LLM key 或额度不足时无法验证采集 | 增加 `pulse --ingest-only` 和 `AIHOT_PULSE_INGEST_ONLY=1` | 使用已充值额度重跑真实 LLM；同时记录空响应、402、拒答和结构化解析失败 |
| LLM 输出格式不稳定 | category 别名归一、只取第一个合法 JSON、分数 clamp、单条失败隔离 | 重跑真实 LLM，确认降级和重试指标 |
| GitHub API 可能限流或截断 | Token 策略、分页上限、间隔、运行记录和 partial 错误类别已落地 | 配置新 Token 后用小页数重跑，确认失败可继续且不重复入库 |
| 定时任务可能误写生产库 | 本地 wrapper 强制检查 `aihot_local_prod_clone`，错误 DSN 返回码 3 | 在 Melos 完成正式 schema、Cron 和回滚方案检查；破坏性测试只在隔离库执行 |
| 前端工具表格窄屏挤压 | 固定列宽、操作列布局和 Playwright 布局用例 | 发布后用真实浏览器尺寸复测 |
| Vitest 有异步更新警告 | 当前不影响 114 条通过 | 可在上线后清理 TermPanel 测试里的 `act(...)` 警告，不作为当前上线阻断项 |

## 5. 当前进度和上线评估

截至 2026-10-14：
- 代码、单测、静态检查、构建、前端自动化和本地真实链路已通过。
- 本地 `aihot_test` 数据库测试已通过，但两个独立测试库的要求尚未满足。
- Playwright 最新回归为 10 条通过、1 条外部源网络失败；本地真实链路为 8/8 通过。
- 真实 LLM 和 GitHub 新凭据尚未完成复测；生产服务、schema 和 Cron 尚未形成证据。
- 339 条用例已经完整导入，当前记录仍为 274 PASS、65 BLOCK，没有新的全量 Spaceway 执行结果。

### 上线评估
**结论：有条件在两天内完成上线准备，但当前不能直接确认可以上线。**
以下上线前检查：
1. 用已充值的 LLM 配置完成一条真实富化、翻译、terms 和热点 smoke；
2. 配置新 GitHub Token，完成一次小页数关键词/Topic/手动添加和 Stars 快照；
3. 固定代理后重跑外部信息源，确认关键来源不再出现可复现的网络错误；
4. 拿到独立测试库，至少完成事务、并发、迁移幂等和失败恢复；
5. 在 Melos 完成生产 schema、服务版本、Cron 和一个受控时间槽检查；
6. 确认回滚方式、监控日志和数据备份可用。
