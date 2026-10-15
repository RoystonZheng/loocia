# 实现状态

最后更新：2026-10-10

## 当前阶段（2026-10-10）

- 工具发现闭环已实现：关键词/Topic/手动添加、GitHub `node_id` 去重、发现来源合并、四状态流转、Cooper URL、团队工具和团队评价。
- 工具运行设置已实现：多 Token 元数据、脱敏预览、轮询/固定/故障转移策略、请求分页/间隔和 Stars 快照参数。
- 发现调度已实现：daily/weekly、北京时间 `HH:MM`、每分钟 `scheduled` 入口、暂停/继续和运行检查点；`weekly` 保留为兼容入口。
- 本地运行已实现：LaunchAgent 保活、cron 调度、本地库 DSN 强校验、无 LLM 凭据时的 ingest-only 采集路径。
- 前端工具页已实现：配置、已发现、测评中、团队工具四个入口；来源/关键词/用途筛选、分页排序、评价、跨页批量和窄屏表格布局。
- 当前自动化结果：前端 Vitest 19 文件/114 测试通过；Playwright mock 9 passed/2 skipped；本地真实验收 8/8 passed；Spaceway 历史全量 339 条为 274 PASS、65 BLOCK。

当前实现仍需注意：Go 数据库测试需要独立 `AIHOT_TEST_DATABASE_URL`/`AI_TOOL_TEST_DATABASE_URL`；GitHub 最近一次 weekly 有 partial；LLM/hotpass 仍有外部额度和返回格式不稳定；生产部署和生产时间槽尚未替代本地验收。

## 已实现

- AI 资讯采集、富化、入库主链路已存在。
- 信息源扩展已接入 `source_kind=rss/html/mp/aihot` 与 `source_role=official/professional/discovery`，并支持配置启停；AI 动态页的来源和主题筛选支持多选。
- 通用来源 MVP 已接入：source plan 支持主题 profile、关键词过滤、RSS/Atom、公开 JSON API 和可配置 CSS selector 的 HTML 列表；`cmd/discover-sources` 会输出带证据的候选来源，分类不再限制为 AI 五类。
- `raw_items` 已持久化 `source_role`，enrichment prompt 已包含来源上下文。
- AIHOT 补漏源已在未处理队列中优先富化，避免被旧 RSS backlog 阻塞；AIHOT 富化不再追溯原网页抓正文。
- 富化解析已支持常见 category 别名归一，并只取模型输出中的第一个合法 JSON 对象，降低单条 LLM 输出异常导致的丢条。
- 新处理资讯的精选门槛已调整为 `relevance >= 4 && score >= 3`。
- 前端 Feed 已支持 RSS/Atom、网页直采、公众号、AIHOT 补漏四类来源筛选。
- 公共 HTTP 入口 `server/cmd/webserver/main.go` 已注册版本、健康检查、资讯列表、日报、热点、图谱和详情页。
- 前端已支持日报、精选、全部动态和图谱视图。
- 部署侧已有 `pulse`、`gendaily`、`hotpass`、`discovertools` 定时任务，`pulse` 可通过 `AIHOT_SOURCES_FILE` 使用外部来源配置。
- AI Cool 2.0 工具发现 PRD 已在本地整理，包含 Cooper 第 5 部分需求、图片、封闭链路 case、接口和前端方案。
- AI Cool 工具发现、测评、团队工具、评价、批量操作、运行设置和 scheduled 调度已经落到当前代码；详细改动、测试结果和问题记录见 [`../handoff/2026-10-10-ai-cool-development-report.md`](../handoff/2026-10-10-ai-cool-development-report.md)。

## 部分实现或历史方案

- `docs/design/ai-cool-2.0-product-design.md` 和原型记录过工具雷达、评测和工具箱方向。
- `docs/design/ai-cool-2.0-product-technical-plan/README.md` 是历史技术方案，其中“首期只读、Git Markdown 报告导入”的口径已被本次 PRD 覆盖。
- `docs/superpowers/` 下存在历史 specs/plans，只用于追溯，不直接作为当前实现事实。

## 未实现或未闭环

- 独立数据库环境下的事务回滚、并发、旧 schema 迁移、审计和故障注入验收。
- 生产环境的正式表变更、生产 cron 首次到点、生产数据恢复和真实生产回滚演练。
- GitHub partial/rate limit 的稳定运行和全量结果质量确认。
- LLM 额度、空响应、翻译拒答和单条 category 异常的生产级稳定性。

## 当前风险

- 外部信息源存在限流、网络失败和 HTML 结构漂移风险；单源失败不会中断其他来源，但需要观察 `pulse` 日志中的 `srcerrs` 及其后续 `source error: <来源>` 明细。
- AIHOT 公共 API 有明确授权边界，外部商业复用或公开再分发需单独授权；当前仅按组织内部补漏使用。
- LLM 富化仍可能出现单条失败；当前按单条隔离和超时处理，不阻断整轮 `pulse`。
- 本次工具需求引入写操作，但首期不接 SSO/RBAC，只靠手填操作人留痕；公网部署前需要重新评估写接口开放范围。
- GitHub API 有限流和搜索上限，需要 token、分页上限、时间窗口拆分和失败记录。
- Cooper 链接首期只校验域名，无法证明文档真实存在或有权限访问。

## 下一步

1. 配置独立测试库 DSN，补跑被 SKIP 的 Go 数据库测试和破坏性验收。
2. 稳定 LLM/GitHub 外部依赖后，重跑 pulse、hotpass、weekly/scheduled，并记录运行结果。
3. 在 Melos 放置 `/root/aihot/etc/sources.json`，用 `AIHOT_SOURCES_FILE` 做信息源 smoke，确认 raw 入库、精选门槛和 `/api/public/items?source_kind=aihot&mode=all`。
4. 生产上线前执行 schema、cron、回滚和数据恢复演练。
