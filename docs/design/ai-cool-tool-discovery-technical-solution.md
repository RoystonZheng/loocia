# AI Cool 工具发现技术方案（轻量版）

| 项目 | 内容 |
|---|---|
| 状态 | 技术评审稿，已补齐上线确认口径 |
| 日期 | 2026-09-10 |
| 需求来源 | Cooper 文档《AI Cool 升级方案》第 5 部分 |
| 代码口径 | 复用现有 AI Cool 服务、数据库和定时系统；新增工具发现域 |

## 1. 需求

### 1.1 本期目标

本期在 AI Cool 里新增“工具发现”模块，完成从发现 GitHub 工具到纳入团队工具的完整链路。

```text
发现配置 / 手动添加 GitHub 仓库
  -> 生成候选工具
  -> 按 GitHub repository node_id 去重并合并来源
  -> 已发现工具
  -> 发起测评，手填测评人和操作人
  -> 关联外部 Cooper 测评文档链接
  -> 完成测评：纳入团队工具 / 不纳入
  -> 团队工具页只展示已纳入工具
```

本期以 Cooper 第 5 部分为准。旧方案里的“首期只读”“Git Markdown 报告导入”“GitHub Trending 自动发现”不作为本期范围。

### 1.2 Cooper 内容

以下内容来自 Cooper 第 5 部分的本地快照，用来固定需求边界。

> “工具只有四个内部状态”

状态只保留 `discovered`、`evaluating`、`included`、`excluded`。不做 `observe`。

> 原型图已内嵌到 Cooper 文档对应位置：工具发现整体流程。

> “发现配置只管理关键词和 Topic”

发现配置页首期上线，支持新增、编辑、启停、手动执行和删除。删除使用软删除：普通列表不再展示，不能继续执行或编辑，但历史运行、发现来源和工具记录保留。

> 原型图已内嵌到 Cooper 文档对应位置：发现配置。

> “AI Cool 不建设站内测评编辑器”

Cooper 测评文档由用户自己创建和维护。AI Cool 只保存 Cooper URL，只校验链接属于预期 Cooper 域名，不创建文档、不读取正文、不校验权限。

> 原型图已内嵌到 Cooper 文档对应位置：测评中。

页面原型还包括已发现工具、手动添加和团队工具。

> 原型图已内嵌到 Cooper 文档对应位置：已发现工具。

> 原型图已内嵌到 Cooper 文档对应位置：手动添加。

> 原型图已内嵌到 Cooper 文档对应位置：团队工具。

### 1.3 已确认的澄清

- 第 5 部分是本期准口径。
- 测评人和操作人先由用户手填。
- Cooper 测评文档由用户自己建，AI Cool 只保存链接。
- 发现配置页首期给前端用户使用。
- 配置删除首期支持，但只能软删除。
- 新功能页面和配置命名使用 `AI Tool` / `AI_TOOL_*`，不沿用旧资讯站命名。
- 生产数据放现有 AI Cool 数据库，不单独建库，只新增工具发现相关表。
- 生产表由开发在上线前按 schema 手动建好；服务启动不把自动 DDL 当上线依赖，`EnsureSchema` 只作为本地、测试和幂等兜底。
- GitHub API token 首期使用个人账号 token，由用户提供；额度按少量配置和手动添加使用，限流时写入运行记录。
- 定时发现复用 AI Cool 现有早间资讯抓取同一套调度系统；任务执行到期的每周配置和 Stars 快照。
- 首期写操作和 `AI Tool` 入口只面向自己使用，不做灰度，也不把额外开关作为上线门槛。
- Cooper 链接点击后直接跳转 Cooper；AI Cool 不验证文档能否打开，也不验证权限。没有权限时由 Cooper 自己处理申请。
- 测试库和测试表在上线前清理，正式库再按本期 schema 建需要的表。
- 上线验证允许用一条关键词配置、一条 Topic 配置、一个手动 GitHub 仓库跑闭环。

### 1.4 首期页面范围

| 页面 | 入口 | 首期能力 |
|---|---|---|
| 发现配置 | `#tools-configs` | 配置列表、新建、编辑、启停、软删除、手动执行 |
| 已发现工具 | `#tools-discovered` | 候选列表、搜索、来源筛选、排序、手动添加、开始测评、不处理 |
| 测评中 | `#tools-evaluating` | 查看测评人、操作人、Cooper 链接，补链接，完成测评 |
| 团队工具 | `#tools-team` | 只展示 `included` 工具，支持搜索和基础统计 |

## 2. 实现方案

### 2.1 后端 + 接口

新增独立工具域，不复用现有资讯 `items` 表。工具有自己的仓库身份、发现来源、状态历史、测评记录和 Stars 快照。

```text
server/internal/tools
  -> schema.sql
  -> Store
  -> GitHub client
  -> discovery runner
  -> status/evaluation logic

server/internal/publicapi
  -> /api/tools/* handlers

server/cmd/discovertools
  -> run-scheduled
  -> run-config
  -> snapshot
```

后端沿用当前仓库方式：

- Go + PostgreSQL + `pgxpool`。
- 模块内 `schema.sql + EnsureSchema`。
- 公共接口放在 `server/internal/publicapi`。
- 两个 HTTP 入口都注册工具接口：`server/cmd/webserver/main.go`、`server/common/server/httpserv/httpserv.go`。
- `discovertools` 负责执行到期配置、指定配置执行和 Stars 快照，并挂到现有 AI Cool 早间定时任务系统。

核心表：

| 表 | 用途 |
|---|---|
| `tool_discovery_configs` | 发现配置，保存关键词或 Topic、触发方式、启用状态、软删除状态 |
| `tool_discovery_runs` | 每次执行记录，保存结果数、失败原因、限流和截断信息 |
| `tools` | 工具主表，按 GitHub `node_id` 唯一去重 |
| `tool_discovery_sources` | 工具来源，记录关键词、Topic 或手动添加 |
| `tool_evaluations` | 测评记录，保存测评人、操作人、Cooper URL 和结论 |
| `tool_status_events` | 状态变化审计 |
| `tool_star_snapshots` | Stars 快照，用于计算近 7 天增长 |

后端环境变量：

| 变量 | 说明 |
|---|---|
| 现有 AI Cool 数据库连接 | 生产复用当前服务数据库连接；工具数据和现有内容数据在同一个库，只新增表 |
| `AI_TOOL_TEST_DATABASE_URL` | 本地和 CI 测试库，不连生产库 |
| `AI_TOOL_GITHUB_TOKEN` | GitHub API token，首期使用个人账号 token，由用户提供 |
| `AI_TOOL_GITHUB_MAX_PAGES` | 单个 GitHub 查询最多翻页数 |

生产库上线前先手动执行正式 schema，建好 `tool_*` 相关表。服务启动可以保留 `EnsureSchema` 的本地和测试兜底能力，但生产发布不能依赖启动时自动 DDL。

接口统一放在 `/api/tools/*`：

| 接口 | 用途 |
|---|---|
| `GET /api/tools/configs` | 配置列表 |
| `POST /api/tools/configs` | 新建或编辑配置 |
| `POST /api/tools/configs/enable` | 启停配置 |
| `POST /api/tools/configs/delete` | 软删除配置 |
| `POST /api/tools/configs/run` | 手动执行配置 |
| `GET /api/tools/items` | 工具列表，按状态返回已发现、测评中、团队工具 |
| `POST /api/tools/manual/preview` | 预览 GitHub 仓库 |
| `POST /api/tools/manual/add` | 手动加入候选 |
| `POST /api/tools/evaluations/start` | 开始测评 |
| `POST /api/tools/evaluations/cooper-url` | 关联或更新 Cooper URL |
| `POST /api/tools/evaluations/finish` | 完成测评 |
| `POST /api/tools/discovered/exclude` | 已发现阶段不处理 |

写接口必须带 `actor`。开始测评必须带 `evaluator`。完成测评前必须已有 Cooper URL。

常用错误码：

| error | 场景 |
|---|---|
| `invalid_github_url` | GitHub URL 不合法 |
| `invalid_cooper_url` | Cooper URL 不合法 |
| `invalid_config` | 配置字段不合法 |
| `status_conflict` | 状态已变化，当前操作不能继续 |
| `active_evaluation_exists` | 工具已有未完成测评 |
| `github_rate_limited` | GitHub API 限流 |
| `github_error` | GitHub API 异常 |

### 2.2 前端 + 接口

前端继续用当前 React + Vite + TypeScript，不引入新路由库。页面按 hash 切换，侧边栏新增 `AI Tool` 分组。

建议新增：

```text
web/src/api/tools.ts
web/src/components/ToolsView.tsx
web/src/components/ToolsView.test.tsx
web/src/api/tools.test.ts
```

页面和接口关系：

| 页面 | 主要接口 |
|---|---|
| 发现配置 | `GET /api/tools/configs`、`POST /api/tools/configs`、`POST /api/tools/configs/enable`、`POST /api/tools/configs/delete`、`POST /api/tools/configs/run` |
| 已发现工具 | `GET /api/tools/items?status=discovered`、`POST /api/tools/manual/preview`、`POST /api/tools/manual/add`、`POST /api/tools/evaluations/start`、`POST /api/tools/discovered/exclude` |
| 测评中 | `GET /api/tools/items?status=evaluating`、`POST /api/tools/evaluations/cooper-url`、`POST /api/tools/evaluations/finish` |
| 团队工具 | `GET /api/tools/items?status=included` |

前端状态：

- 加载态：固定高度 loading 或 skeleton。
- 空态：保留下一步入口，比如“手动添加”“新建发现配置”。
- 错误态：展示失败原因和重试按钮。
- 操作态：当前按钮禁用，成功后刷新列表。
- 冲突态：收到 `status_conflict` 后刷新列表，提示工具状态已变化。

## 3. 规则和边界

### 3.1 状态规则

状态只能这样变：

```text
discovered -> evaluating
discovered -> excluded
evaluating -> included
evaluating -> excluded
```

GitHub 再次命中已有工具时，只更新仓库事实、来源和最近发现时间，不自动改状态。

### 3.2 去重规则

- GitHub `node_id` 是唯一身份。
- 同一仓库被关键词、Topic、手动添加重复命中，只保留一条 `tools` 记录。
- 来源写入 `tool_discovery_sources`。
- 仓库改名或 URL 变化不创建新工具。

### 3.3 发现配置规则

- 一条配置只允许一种发现方式：关键词或 Topic。
- 高级 GitHub 查询参数首期不暴露给前端用户。
- 默认只处理公开仓库、排除归档仓库、默认排除 Fork。
- 配置删除是软删除，不删除历史结果。

### 3.4 GitHub 发现规则

GitHub 发现只用官方 API：

| 场景 | API |
|---|---|
| 关键词 / Topic 扫描 | `GET /search/repositories` |
| 手动添加预览 | `GET /repos/{owner}/{repo}` |
| Stars 快照 | `GET /repos/{owner}/{repo}` |

关键词查询示例：

```text
"browser agent" in:name,description,topics,readme fork:false archived:false
```

Topic 查询示例：

```text
topic:browser-automation fork:false archived:false
```

执行规则：

- 首次执行做有限页数基线扫描。
- 后续按 `last_success_at - 24h` 做增量。
- 增量同时查 `created:>` 和 `pushed:>`。
- 记录 `incomplete_results`、`truncated`、rate limit reset 时间和 bad query。
- 单条配置失败不影响其他配置。

### 3.5 Cooper 规则

- AI Cool 不创建 Cooper 文档。
- AI Cool 不读取 Cooper 正文。
- AI Cool 不校验 Cooper 文档权限。
- AI Cool 只保存 Cooper URL，点击时直接跳转 Cooper。
- 保存时只做基本 URL 和 Cooper 域名判断，不调用 Cooper 接口确认可打开或有权限。
- 完成纳入或不纳入前必须已有 Cooper URL。

### 3.6 Stars 规则

- 近 7 天增长只来自 `tool_star_snapshots`。
- 没有 6 到 8 天前的可用快照时展示“暂无”。
- Stars 增长只作为列表展示和排序信号，不产生新工具，不叫 GitHub Trending。

## 4. 开发计划和每个阶段验证方式

| 阶段 | 开发内容 | 验证方式 |
|---|---|---|
| 1. 数据模型 | 新增 `server/internal/tools`、schema、类型、Store、状态事件 | 本地 PostgreSQL 建表；验证 `node_id` 去重、来源合并、软删除、状态事务 |
| 2. GitHub 发现 | GitHub client、关键词/Topic 查询、分页、限流、`discovertools` 基础命令 | fake GitHub server 覆盖 200、403、422、5xx、分页截断；本地执行一次配置 |
| 3. 测评流转 | 开始测评、补 Cooper URL、纳入、不纳入、不处理 | 单测覆盖合法流转、非法状态、重复开始测评、缺 Cooper URL 完成失败 |
| 4. 后端接口 | `/api/tools/*` handlers，接入两个 HTTP 入口 | handler 测试覆盖字段校验、错误码、状态过滤；本地请求接口返回正常 |
| 5. 前端页面 | `AI Tool` 导航、四个页面、弹窗、列表刷新和错误态 | `npm run build`、`npm run lint`、`npx vitest run`；本地页面跑通主路径 |
| 6. 本地完整链路 | 配置发现或手动添加 -> 测评 -> 纳入 -> 团队工具可见 | 使用本地 PostgreSQL 跑一条关键词配置、一条 Topic 配置、一个手动 GitHub URL |
| 7. 部署准备 | 清理测试表和测试数据、手动建正式表、打包 `discovertools`、接入现有早间调度 | 确认正式库新表存在；定时任务 dry run；`AI Tool` 自用入口可访问 |

上线前至少执行：

```bash
go test -p 1 ./...
npm run build
npm run lint
npx vitest run
```

当前验证先用本地 PostgreSQL。上线前删除测试表和测试数据，在现有 AI Cool 生产库按正式 schema 建新表，再用允许的三条数据跑线上验证。

## 5. 上线前需要确认的内容

### 5.1 已确认

- 数据库：不单独开库，放现有 AI Cool 数据库；上线前我们自己把新表建好，不靠应用启动自动建表。
- 测试表：上线前删掉测试用表和测试数据，再按正式 schema 建需要的表。
- GitHub Token：先用个人账号 token；用户后续提供。代码只从配置读取，不写进仓库。
- 定时任务：工具发现和 Stars 快照接入 AI Cool 现有早间资讯抓取同一套调度系统。
- 使用范围：首期自己用，`AI Tool` 入口不做灰度。
- Cooper 链接：AI Cool 只保存和跳转，不校验打开权限；没权限时由 Cooper 页面处理申请。
- 上线验证：允许用一条关键词配置、一条 Topic 配置、一个手动 GitHub 仓库跑完整链路。

### 5.2 上线前还要落地

- 拿到 `AI_TOOL_GITHUB_TOKEN` 并放到运行环境配置。
- 确认现有调度系统里新增 `discovertools` 命令的具体运行时间和日志位置。
- 确认失败是否只看页面运行记录；首期默认不做 IM 或邮件通知。
