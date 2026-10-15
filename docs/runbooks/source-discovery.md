# 通用来源发现与接入

## 适用场景

为一个新的主题配置 RSS/Atom、公开 JSON API 或 HTML 列表来源。

## 步骤

1. 准备主题描述、关键词、分类和官方站点入口。凭证只放在运行环境变量。
2. 按根目录 [`CODEX.md`](../../CODEX.md) 的命令运行 `go run ./cmd/discover-sources`。
3. 逐条检查候选的 URL、响应类型和证据。优先选 RSS/Atom；API 需要确认列表字段；HTML 需要确认 selector 在页面结构中稳定。
4. 写入 `source-plan.json`，用 `header_env` 引用凭证环境变量名。
5. 用 `go run ./cmd/pulse -sources source-plan.json -ingest-only` 验证 raw 入队，再按现有数据库和 LLM 配置运行完整 pulse。

## 失败处理

- 没有候选：确认种子 URL 可从运行环境访问，再手动查找官方文档中的 feed/API 入口。
- API 返回 401/403：不要把凭证写入仓库；设置 `header_env` 对应环境变量并确认权限。
- HTML 没有条目：缩小 `item_selector`，并确认 `link_selector`、`title_selector` 能在实际 DOM 中匹配。
- 来源响应很大或频繁限流：降低 `max_items_per_run`，设置 `timeout_seconds`/`rate_limit`，优先改用官方 feed/API。
