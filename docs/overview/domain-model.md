# 业务模型

## 当前资讯领域

| 实体 | 代码位置 | 说明 |
|---|---|---|
| RawItem | `server/internal/ingest` | 外部来源采集到的原始条目 |
| Item | `server/internal/items` | 富化后的公开内容，供列表、详情和图谱使用 |
| Daily | `server/internal/daily` | 日报数据 |
| HotTopic | `server/internal/cluster` | 热点聚类结果 |
| Term | `server/internal/terms` | 词云和词条相关内容 |

当前主链路是 `raw_items -> pipeline -> items`。工具发现不复用 `RawItem` 或 `Item`，因为工具需要去重、来源合并、状态流转和测评记录。

## 信息源扩展模型

`SourceConfig` 位于 `server/internal/pulse`，负责把来源配置翻译为 `ingest.Source`。旧格式 `{name,url}` 仍按 RSS 处理；新格式支持 `adapter`、`source_kind`、`source_role`、`enabled`、单轮上限、二手线索上限、超时和 `rate_limit`。

`RawItem` 新增 `SourceRole`，仅在 `raw_items` 和 enrichment 阶段使用，不进入公开 `items` DTO。

| 字段 | 枚举 | 含义 |
|---|---|---|
| `source_kind` | `rss`、`html`、`mp`、`aihot` | 采集方式，用于来源筛选和部分处理分支 |
| `source_role` | `official`、`professional`、`discovery` | 富化提示词的阅读价值判断口径 |

来源角色规则：

- `official`：官方博客、官网新闻或明确登记的官方公众号。官方身份不自动加分，活动、招聘、预告和空洞宣传仍低分。
- `professional`：科技媒体、专家博客和普通公众号。更看重原创采访、实测、代码、数据、工程经验和独特分析。
- `discovery`：社区、聚合和补漏来源。传闻、截图、单一反馈和二手摘要要压低，AIHOT 也属于该角色。

精选规则只影响新处理内容：`selected = relevance >= 4 && score >= 3`，`ai_selected` 与该门槛同步。历史 `items.selected` 不批量回刷。

## 本次新增工具领域

| 实体 | 说明 |
|---|---|
| ToolDiscoveryConfig | 发现配置，包含名称、方式、关键词或 Topic、触发方式、启用状态和最近执行信息 |
| ToolDiscoveryRun | 一次发现任务执行记录，保存触发来源、操作者、命中数量、失败原因和 GitHub API 限制信息 |
| Tool | GitHub 仓库级工具实体，以 GitHub `node_id` 去重 |
| ToolDiscoverySource | 工具被哪些配置、关键词、Topic 或手动入口命中过 |
| ToolEvaluation | 测评记录，保存测评人、Cooper 链接、开始/完成时间、结论和确认后的一句话介绍 |
| ToolStatusEvent | 状态变更流水，保存操作人、阶段和原因 |
| ToolStarSnapshot | Stars 快照，用于计算近 7 天增长 |

## 工具状态

```text
discovered
  -> evaluating
  -> included

discovered
  -> excluded

evaluating
  -> excluded
```

- `discovered`：已发现，尚未决定是否测评。
- `evaluating`：已选择测评，正在形成 Cooper 记录。
- `included`：测评完成并纳入团队工具列表。
- `excluded`：不处理或测评后不纳入；保留记录用于去重，但不出现在主列表。

## 页面和路由

当前前端使用 hash 视图：

- `/`：日报首页。
- `#all`：AI 动态主视图，默认展示全部可展示资讯。
- `#selected`：兼容旧链接，进入 AI 动态并默认选中“精选”筛选。
- `#graph`：图谱。

本次工具模块规划新增：

- `#/tools/discovered`：工具百宝箱。
- `#/tools/team`：团队工具。
- `#/tools/configs`：发现配置。

`evaluating` 仍是工具生命周期中的后端状态，用于驱动测评写入和流转；当前前端不把它作为独立导航入口展示。

## 关键规则

- 资讯 raw 入队以规范化 URL hash 去重：scheme/host 小写，去掉默认端口、fragment、常见追踪参数，排序 query，并清理路径中的 `.`/`..`；AIHOT 有原文链接时用原文 URL，无原文链接时仅在二手线索上限内使用 `aihot:<id>` 派生稳定 ID。
- `rate_limit` 支持 Go duration（如 `250ms`、`2s`）或 `count/window`（如 `10/m`），加载来源时会包裹为单来源抓取前的最小等待时间。
- `source_role` 不对外展示，前端只通过 `source_kind` 做来源筛选，通过 `source` 文案展示来源名。
- GitHub 仓库以 `node_id` 唯一。
- 重复命中时合并来源并刷新最近发现时间，不重复创建工具。
- `included` 的一句话介绍以测评人确认值为准。
- Cooper 链接只校验域名，首期不读取正文。
- 所有写操作都要求用户手填操作人。
