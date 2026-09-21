# AI Cool Tools API

本文件记录工具发现与测评闭环的前后端接口契约。所有接口挂在 `/api/tools/` 下，写操作使用 `POST + JSON body`，响应统一为：

```json
{
  "errno": 0,
  "errmsg": "ok",
  "data": {}
}
```

非 0 `errno` 表示失败。常见错误：`400001` 参数错误，`404001` 资源不存在，`409001` 状态冲突，`429001` GitHub 限流，`502001` GitHub 或网络错误，`500001` 服务内部错误。错误响应的 `data.class` 会返回内部错误分类，如 `validation_error`、`invalid_github_url`、`invalid_cooper_url`、`missing_cooper_url`、`status_conflict`。

## 配置

### GET `/api/tools/configs`

查询发现配置。参数：

| 字段 | 说明 |
|---|---|
| `q` | 可选，按配置名、检索词、发现方式或触发方式搜索 |
| `method` | 可选，可重复，`keyword` 或 `topic`；不传表示全部发现方式 |
| `triggerMode` | 可选，可重复，`manual` 或 `weekly`；不传表示全部触发方式 |

前端使用两个下拉筛选：发现方式为“全部 / 关键词 / Topic”，触发方式为“全部 / 手动执行 / 自动执行”。选择“全部”时不传对应参数。

返回 `data.items[]`：`id`、`name`、`method`、`terms`、`triggerMode`、`enabled`、`lastSuccessAt`、`lastRunAt`、`lastRunStatus`、`lastResultCount`、`lastPagesScanned`、`lastPauseRequested`、`lastFailureReason`、`createdBy`、`updatedBy`、`createdAt`、`updatedAt`。

返回统计：`count`、`enabledCount`、`disabledCount`。

### POST `/api/tools/configs`

新建或编辑发现配置。Body：

| 字段 | 说明 |
|---|---|
| `id` | 可选；为空表示新建，有值表示编辑 |
| `name` | 必填，配置名称 |
| `method` | 必填，`keyword` 或 `topic` |
| `terms` | 必填，检索词或 Topics；会去重 |
| `triggerMode` | 可选，`manual` 或 `weekly`，默认 `manual` |
| `enabled` | 可选，是否启用；新建默认启用，编辑默认沿用原值 |
| `actor` | 新建必填，编辑已有配置时为空会沿用原配置的 `updatedBy/createdBy` |

限制：关键词不允许包含 GitHub 高级限定符；Topic 只允许小写字母、数字和连字符。删除后的配置不能编辑。

### POST `/api/tools/configs/enable`

启用或停用配置。Body：`id`、`enabled`、`actor`。已有配置的 `actor` 为空时会沿用原配置的 `updatedBy/createdBy`。

### POST `/api/tools/configs/delete`

软删除配置。Body：`id`、`actor`。已有配置的 `actor` 为空时会沿用原配置的 `updatedBy/createdBy`；有运行中或已暂停的任务时返回 `409001/status_conflict`。

### POST `/api/tools/configs/run`

手动启动配置执行。Body：`id`、`actor`。已有配置的 `actor` 为空时会沿用原配置的 `updatedBy/createdBy`；同一配置已有运行中或已暂停任务时返回 `409001/status_conflict`。

接口只负责创建 run 并立即返回 `status=running`，实际 GitHub 检索在服务端后台继续执行，不依赖浏览器请求。用户刷新页面不会打断本次执行，前端通过重新拉取 `GET /api/tools/configs` 展示 `lastRunStatus`、`lastPagesScanned` 和 `lastResultCount`。

返回 `data.run`：`status`、`resultCount`、`newCount`、`updatedCount`、`skippedCount`、`pagesScanned`、`incompleteResults`、`truncated`、`pauseRequested`、`nextQueryIndex`、`nextPage`、`errorClass`、`errorMessage`、`rateLimited`、`rateLimitResetAt`。

### POST `/api/tools/configs/pause`

请求暂停当前配置的执行。Body：`id`、`actor`。已有配置的 `actor` 为空时会沿用原配置的 `updatedBy/createdBy`。接口会把当前 active run 标记为 `pauseRequested=true`，后台任务在下一次页级检查点落成 `status=paused`，并保留 `nextQueryIndex`、`nextPage` 作为续跑位置。没有运行中任务时返回 `409001/status_conflict`。

### POST `/api/tools/configs/resume`

继续已暂停的配置执行。Body：`id`、`actor`。已有配置的 `actor` 为空时会沿用原配置的 `updatedBy/createdBy`。接口把 paused run 改回 `running` 并从上次保存的 `nextQueryIndex`、`nextPage` 继续跑；没有已暂停任务时返回 `409001/status_conflict`。

## 工具列表

### GET `/api/tools/items`

查询工具列表。参数：

| 字段 | 说明 |
|---|---|
| `status` | 可选，`discovered`、`evaluating`、`included`、`excluded`；默认 `discovered` |
| `sort` | 可选，`latest`、`stars`、`stars7d`；默认 `latest` |
| `source` | 可选，可重复，`keyword`、`topic`、`manual`；不传表示全部来源 |
| `purposeTag` | 可选，可重复，按用途分类筛选；不传表示全部用途 |
| `q` | 可选，按工具、仓库、来源、测评人或 Cooper 链接搜索 |
| `take` | 可选，1-200，默认 50 |
| `page` | 可选，从 1 开始的页码；不传默认第 1 页 |
| `offset` | 可选，从 0 开始的偏移量；如果同时传 `offset` 和 `page`，后端优先使用 `offset` |

返回 `data.count` 是同一筛选条件下的工具总数，不受 `take`、`page`、`offset` 和排序方式影响；`data.items[]` 是当前排序下的当前页。返回里同时带 `page`、`pageSize`、`offset`、`take`，前端用它展示“第几条到第几条 / 共多少条”。发现工具、团队工具在切换 `latest`、`stars`、`stars7d` 时，统计口径保持一致，但列表行会按排序规则取当前页。

返回 `data.stats`：`keywordSourceCount`、`topicSourceCount`、`manualSourceCount`、`linkedEvaluationCount`、`unlinkedEvaluationCount`、`evaluatorCount`、`latestUpdatedAt`、`purposeTags`。这些统计和 `count` 一样按完整筛选集合计算；`purposeTags` 是当前状态、搜索和来源条件下可选的用途分类集合，不受当前用途筛选影响。

返回 `data.items[]`：GitHub 仓库事实、当前状态、当前说明、`summaryKey`、Stars、`stars7d`、用途分类 `purposeTags`、是否人工修正 `purposeTagsManuallySet`、发现来源 `sources[]`、测评摘要 `evaluation`。`stars7d` 优先使用 7 天前附近的快照计算；没有 7 天前快照时，用最近 7 天内最早且后续已有更新的快照计算；完全没有历史快照时为空。已发现、测评中、团队工具三个列表都使用同一套 `q`、`sort`、`source`、`purposeTag` 查询能力，其中团队工具页读取 `status=included`，测评中页读取 `status=evaluating`。

前端来源筛选使用下拉菜单：全部来源、关键词、Topic、手动添加。选择“全部来源”时不传 `source`。

前端用途筛选同样使用下拉菜单：全部用途、代码开发、浏览器操作、深度研究等。选择“全部用途”时不传 `purposeTag`。如果自动分类没有命中，工具可以保持未分类；用户可以在列表里手动修正用途，也可以新增一个当前没有的用途标签。

### POST `/api/tools/purpose-tags`

手动修正工具用途分类。Body：

| 字段 | 说明 |
|---|---|
| `toolId` | 必填，工具 ID |
| `actor` | 必填，操作人 |
| `purposeTags` | 可选，字符串数组；为空数组表示人工确认未分类 |

提交成功后返回更新后的 `tool`，其中 `purposeTagsManuallySet=true`。后续自动发现再次命中该工具时，不会覆盖人工修正过的用途分类。

### 前端批量操作

首期不新增后端批量接口。前端列表勾选多个工具后，按所选工具逐条调用现有单条接口，单条成功即落库，单条失败会在批量弹窗中展示到具体仓库。翻页不会清空已选工具，批量操作按跨页选择的完整集合执行；切换页面入口、搜索、排序或来源筛选时会清空选择，避免把不同筛选条件下的工具混在一次批量操作里。

| 页面 | 批量动作 | 调用接口 |
|---|---|---|
| 工具百宝箱 | 批量测试 | 对每个工具调用 `POST /api/tools/evaluations/start`；测评人和操作人共用，Cooper 链接逐条填写且可为空 |
| 工具百宝箱 | 批量删除 | 对每个工具调用 `POST /api/tools/discovered/exclude`；操作人和删除原因共用 |
| 测评中 | 批量纳入 | 如该工具还没有 Cooper 链接，先调用 `POST /api/tools/evaluations/cooper-url`；再调用 `POST /api/tools/evaluations/finish`，每个工具单独填写团队使用说明 |
| 测评中 | 批量不纳入 | 对每个工具调用 `POST /api/tools/evaluations/finish`，`result=excluded`；每个工具单独填写不纳入原因 |
| 团队工具 | 批量修改 | 对每个工具调用 `POST /api/tools/team/update`；操作人共用，Cooper 文档和团队使用说明逐条填写 |
| 团队工具 | 批量删除 | 对每个工具调用 `POST /api/tools/team/delete`；操作人和删除原因共用 |

批量纳入前，前端会校验：操作人必填；Cooper 链接必须已存在或本次填写；团队使用说明必须 20-1000 字。团队工具批量删除仍是软删除，状态变为 `excluded`，保留来源、测评和状态事件。

### POST `/api/tools/summaries/url-key`

按工具的 `summaryKey` 获取中文简介。Body：`toolId`、`urlKey`。后端会校验 `urlKey` 是否匹配该工具的 GitHub URL。

返回 `data.summary`、`data.source`、`data.translated`。如果工具已经有最终团队说明、已有临时中文说明或 GitHub 描述本身是中文，直接复用；如果只有英文 GitHub 描述，会先短时间尝试用服务端摘要生成器生成中文简介并写回 `temporary_summary`，来源记录为 `url_key:<summaryKey>`。摘要生成器未配置、超时、失败或返回非中文时，接口会立即使用仓库名、Topics 和描述关键词生成一条本地中文兜底简介，来源记录为 `url_key_fallback:<summaryKey>`，避免列表 tooltip 因外部模型慢而加载不稳定。

## 手动添加

### POST `/api/tools/manual/preview`

预览 GitHub 仓库，不入库。Body：`repoUrl`。返回 GitHub 仓库事实，用于前端确认卡片。

### POST `/api/tools/manual/add`

确认添加 GitHub 仓库。Body：`repoUrl`、`actor`、可选 `purposeTags`。提交时按 GitHub `node_id` 去重，重复仓库会合并 `manual` 来源并返回已有工具。如果 `purposeTags` 为空，后端优先用 AI 按仓库名、描述、Topics 和已有说明推断用途；AI 不可用或失败时用本地规则兜底。如果前端传入用途标签，则视为人工修正，不再被自动发现覆盖。

## 团队工具

团队工具是 `status=included` 的工具。前端通过 `GET /api/tools/items?status=included` 查询，支持同样的 `q`、`sort`、`source` 和 `take` 参数。搜索覆盖工具名、仓库名、描述、团队使用说明、测评人、Cooper 链接和发现来源关键词。

### POST `/api/tools/team/import`

手动导入团队工具。Body：

| 字段 | 说明 |
|---|---|
| `repoUrl` | 必填，公开 GitHub 仓库地址 |
| `operator` | 必填，操作人 |
| `cooperUrl` | 可选，Cooper 文档链接；只校验域名 |
| `finalSummary` | 可选，团队使用说明，最多 1000 字 |
| `purposeTags` | 可选，用途分类；为空时后端优先 AI 推断，失败时本地规则兜底 |

提交时按 GitHub `node_id` 去重。新仓库会写入工具表、手动来源、Star 快照、已完成测评记录和状态事件；已存在仓库会更新为团队工具并追加操作记录。

### POST `/api/tools/team/update`

编辑团队工具。Body：`toolId`、`operator`、可选 `cooperUrl`、可选 `finalSummary`。仅允许编辑已纳入团队工具；每次编辑都会新增一条已完成测评记录，并写入状态事件，便于追溯是谁更新了文档或说明。

### POST `/api/tools/team/delete`

删除团队工具。Body：`toolId`、`operator`、`reason`。`operator` 和 `reason` 必填。删除是软删除：工具状态从 `included` 变为 `excluded`，原仓库、来源、测评和状态事件保留，删除原因写入 `excluded_reason` 和状态事件。

## 测评

### POST `/api/tools/evaluations/start`

从已发现进入测评中。Body：`toolId`、`evaluator`、`operator`、可选 `cooperUrl`。只能从 `discovered` 触发。

### POST `/api/tools/evaluations/cooper-url`

关联或更新 Cooper 测评链接。Body：`toolId`、`cooperUrl`、`operator`。只校验 URL 属于 `cooper.didichuxing.com`，不创建、不读取、不校验 Cooper 文档权限。

### POST `/api/tools/evaluations/finish`

完成测评。Body：

| 字段 | 说明 |
|---|---|
| `toolId` | 工具 ID |
| `evaluationId` | 测评记录 ID |
| `result` | `included` 或 `excluded` |
| `operator` | 操作人 |
| `finalSummary` | `included` 时必填，20-1000 字 |
| `notIncludedReason` | `excluded` 时必填 |

完成前必须已有 Cooper 链接。纳入后进入团队工具列表；不纳入后进入 `excluded`，不进入主列表。

### POST `/api/tools/discovered/exclude`

已发现阶段不处理。Body：`toolId`、`reason`、`operator`。只能从 `discovered` 触发。
