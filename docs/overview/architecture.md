# 系统架构

## 系统边界

AI Cool 当前由 Go 后端、PostgreSQL 数据库、React 前端和定时任务组成。后端有两个 HTTP 入口：一个是无内部依赖的 `net/http` 公共入口，一个是 Nuwa 入口。两者需要注册相同的业务 handler。

## 后端模块

| 模块 | 代码位置 | 职责 |
|---|---|---|
| WebServer | `server/cmd/webserver/main.go` | 公网可运行 HTTP 入口 |
| Nuwa HTTP | `server/common/server/httpserv/httpserv.go` | 内部 Nuwa 入口 |
| DB | `server/internal/db` | PostgreSQL 连接池 |
| Ingest | `server/internal/ingest` | 原始资讯采集 |
| Pipeline | `server/internal/pipeline` | 内容富化、翻译和入库 |
| Items | `server/internal/items` | 公开内容存储和查询 |
| Public API | `server/internal/publicapi` | 前端 API handler |
| Daily/Cluster/Terms | `server/internal/daily`、`cluster`、`terms` | 日报、热点和图谱数据 |

## 本次工具模块规划

新增独立工具领域模块，不改造现有资讯表为万能表：

- `server/internal/tools`：工具发现配置、工具实体、来源、测评、状态流水和 Stars 快照的 schema、Store、GitHub client、runner。
- `server/internal/publicapi/tools_public.go`：工具列表读接口。
- `server/internal/publicapi/tools_admin.go`：配置保存、手动执行、手动添加、开始测评、不处理、完成测评。
- `server/cmd/discovertools/main.go`：每周发现、指定配置执行和 Stars 快照刷新。

两个 HTTP 入口都要注册工具 API。部署脚本需要打包 `discovertools` 并接入 cron。

## 前端模块

| 模块 | 代码位置 | 职责 |
|---|---|---|
| App | `web/src/App.tsx` | hash 视图切换和应用壳层 |
| Sidebar | `web/src/components/Sidebar.tsx` | 侧栏导航 |
| Feed/Daily/Graph | `web/src/components` | 当前资讯、日报和图谱页面 |
| API | `web/src/api` | 前端请求封装 |

本次新增 `web/src/api/tools.ts` 和 `web/src/components/tools/*`，沿用当前 React + 自研 CSS 风格，不引入新 UI 组件库。

## 外部依赖

- GitHub REST API：工具发现和手动添加使用官方 API。
- Cooper：仅保存用户填写的 Cooper 测评文档链接，不自动创建或读取正文。
- LLM：当前仅用于资讯富化和详情页重新翻译；工具发现首期不依赖 LLM 做最终测评结论。

## 核心链路

```text
发现配置或手动添加
  -> GitHub API
  -> tools 独立表
  -> 已发现工具页面
  -> 用户开始测评并填写 Cooper 链接
  -> 用户完成测评
  -> included 工具进入团队工具页面
```
