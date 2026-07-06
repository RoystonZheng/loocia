# Runbooks

本目录记录“怎么做”的操作手册，面向本地开发、联调、排障和运维。

## 当前手册

| 文件 | 说明 |
|---|---|
| （按项目填写） | 如 `local-dev.md`：本地开发环境、依赖启动、构建和测试 |
| （前端，按需） | 如 `frontend-verify.md`：启动 / lint / 类型检查 / 组件测试 / 浏览器视觉验证 / 响应式断点检查 |

## 维护规则

- Runbook 要写清楚前置条件、操作命令、预期结果和失败排查。
- 前端验证手册要写清：跑哪些命令（tsc / lint / build / test）、点哪些关键路径、切哪些交互态（空 / 加载 / 错误）、量哪些响应式断点、控制台与网络怎么看。
- 如果某个问题重复出现，把原因和规避方式同步到 [`../pitfalls/README.md`](../pitfalls/README.md)。
- 不在 runbook 中重复完整设计背景；需要背景时链接到 [`../overview/`](../overview/) 或 [`../design/`](../design/)。

## 生产部署

数据管线部署在 Melos 个人开发机（`ssh melos`），由 root crontab 驱动，共三条定时任务：

- `pulse`（每 30 分钟）：抓源 + LLM 富化入库
- `gendaily`（每小时 :10）：生成日报
- `hotpass`（每小时 :20）：热点聚类

生产数据库为 Melos 本机 PostgreSQL 的 `aihot` 库（勿与 `aihot_test` 混淆）。
完整部署与排障手册见仓库根目录 [`deploy/README.md`](../../deploy/README.md)。
