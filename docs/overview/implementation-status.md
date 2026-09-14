# 实现状态

最后更新：2026-09-14

## 已实现

- AI 资讯采集、富化、入库主链路已存在。
- 信息源扩展已接入 `source_kind=rss/html/mp/aihot` 与 `source_role=official/professional/discovery`，并支持配置启停。
- `raw_items` 已持久化 `source_role`，enrichment prompt 已包含来源上下文。
- 新处理资讯的精选门槛已调整为 `relevance >= 4 && score >= 3`。
- 前端 Feed 已支持 RSS/Atom、网页直采、公众号、AIHOT 补漏四类来源筛选。
- 公共 HTTP 入口 `server/cmd/webserver/main.go` 已注册版本、健康检查、资讯列表、日报、热点、图谱和详情页。
- 前端已支持日报、精选、全部动态和图谱视图。
- 部署侧已有 `pulse`、`gendaily`、`hotpass`、`discovertools` 定时任务，`pulse` 可通过 `AIHOT_SOURCES_FILE` 使用外部来源配置。
- AI Cool 2.0 工具发现 PRD 已在本地整理，包含 Cooper 第 5 部分需求、图片、封闭链路 case、接口和前端方案。

## 部分实现或历史方案

- `docs/design/ai-cool-2.0-product-design.md` 和原型记录过工具雷达、评测和工具箱方向。
- `docs/design/ai-cool-2.0-product-technical-plan/README.md` 是历史技术方案，其中“首期只读、Git Markdown 报告导入”的口径已被本次 PRD 覆盖。
- `docs/superpowers/` 下存在历史 specs/plans，只用于追溯，不直接作为当前实现事实。

## 未实现

- 工具发现配置、GitHub 搜索、手动添加和去重入库。
- 工具状态机、测评记录、Cooper 链接保存和团队工具列表。
- 工具相关 API、前端页面、部署命令和 cron。
- 工具模块的数据表和 Store。
- 工具模块测试和端到端验收数据。

## 当前风险

- 外部信息源存在限流、网络失败和 HTML 结构漂移风险；单源失败不会中断其他来源，但需要观察 `pulse` 日志中的 `srcerrs`。
- AIHOT 公共 API 有明确授权边界，外部商业复用或公开再分发需单独授权；当前仅按组织内部补漏使用。
- 本次工具需求引入写操作，但首期不接 SSO/RBAC，只靠手填操作人留痕；公网部署前需要重新评估写接口开放范围。
- GitHub API 有限流和搜索上限，需要 token、分页上限、时间窗口拆分和失败记录。
- Cooper 链接首期只校验域名，无法证明文档真实存在或有权限访问。

## 下一步

信息源扩展上线前需要在 Melos 放置 `/root/aihot/etc/sources.json`，用 `AIHOT_SOURCES_FILE` 启用后跑一次 `pulse` smoke，确认 raw 入库、精选门槛和 `/api/public/items?source_kind=aihot&mode=all`。
