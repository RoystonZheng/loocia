# 技术栈

## 后端

| 维度 | 当前事实 |
|---|---|
| 语言 | Go |
| 本地工具链 | `server/Makefile` 的 `run/test/build` 目标固定 `GOTOOLCHAIN=go1.25.5` |
| HTTP | `net/http` 公共入口 + Nuwa 入口 |
| 数据库 | PostgreSQL |
| 数据库客户端 | `github.com/jackc/pgx/v5/pgxpool` |
| Schema | 各领域内嵌 `schema.sql`，通过 `EnsureSchema` 幂等建表 |
| 定时任务 | `pulse`、`gendaily`、`hotpass`，部署侧通过 `deploy/aihot-cron.sh` 调用 |

## 前端

| 维度 | 当前事实 |
|---|---|
| 语言 | TypeScript |
| 框架 | React 19 |
| 构建 | Vite |
| 样式 | 自研 CSS，当前未使用 Ant Design |
| 路由 | `window.location.hash` 驱动视图切换 |
| 测试 | Vitest + Testing Library |
| Lint | oxlint |

## 常用命令

后端：

```bash
cd server
make test
make build
```

前端：

```bash
cd web
npm run build
npm run lint
```

## 运行配置

- `AIHOT_DATABASE_URL`：后端 PostgreSQL DSN。
- `AIHOT_HTTP_ADDR`：`server/cmd/webserver` 监听地址，默认 `:8991`。
- `AIHOT_LLM_*`：可选，启用详情页重新翻译能力。

本次工具发现规划新增：

- `AICOOL_GITHUB_TOKEN`：GitHub API token。
- `AICOOL_GITHUB_MAX_PAGES`：单次发现最大分页数。
- `AICOOL_TOOLS_WRITE_ENABLED`：写接口启用开关。

## 部署

当前部署脚本在 `deploy/` 下，公共服务通过 `deploy/install-serve-melos.sh` 和 supervisor 常驻，数据管线通过 `deploy/install-melos.sh` 写入 cron。工具发现落地后需要把 `discovertools` 纳入打包和 cron。
