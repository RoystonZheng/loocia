# 操作手册

本目录记录项目启动、配置、联调、验证、部署、回滚和排障的具体操作步骤。

## 文件索引

| 文件 | 内容 | 什么时候读取 |
|---|---|---|
| [`../../deploy/README.md`](../../deploy/README.md) | 生产部署、24/7 托管和排障 | 部署或排查线上服务时 |
| `local-dev.md` | 本地环境、启动和测试方法 | 本地开发时 |
| `deployment.md` | 发布、验证和回滚步骤 | 部署系统时 |
| `<问题>-troubleshooting.md` | 某类问题的排查流程 | 出现对应故障时 |

### 平台固定知识

| 文件 | 回答的问题 | 什么时候读取 |
|---|---|---|
| [`rds-schema-change.md`](./rds-schema-change.md) | RDS 表、字段和索引怎样合规变更，如何验收和处理失败 | 创建或修改表、字段、索引时 |
| [`rds-data-query.md`](./rds-data-query.md) | 怎样安全查询测试或生产数据并保护敏感信息 | 核对数据库数据时 |
| [`apollo-config.md`](./apollo-config.md) | Apollo 怎样新增、查看、发布、回滚配置，以及应用怎样接入 | 操作或接入 Apollo 时 |
| [`kms-secret.md`](./kms-secret.md) | KMS 怎样新增、查看、接入和轮转凭证 | 管理敏感配置时 |

固定知识只保存跨项目稳定的规则、所需输入、结果导向步骤、验收、失败处理和回滚。具体项目的服务名、namespace、config、key、库表名、Secret ID、Version、集群、账号、负责人和当前状态不写入这些文件。

## 项目运维摘录

数据管线部署在 Melos 个人开发机（`ssh melos`），由 root crontab 驱动，共三条定时任务：

- `pulse`（每 30 分钟）：抓源 + LLM 富化入库。
- `gendaily`（每小时 :10）：生成日报。
- `hotpass`（每小时 :20）：热点聚类。

生产数据库为 Melos 本机 PostgreSQL 的 `aihot` 库（勿与 `aihot_test` 混淆）。全站 24/7 常驻 Melos：`http://10.190.12.242:8899/`。

## 维护规则

### 命名规则

常规操作按场景命名，如 `local-dev.md`；专项排障文档使用 `<问题>-troubleshooting.md`。

### 新增和更新规则

出现新的独立操作流程时新增文件；已有命令、配置或流程变化时更新原文件。新增、删除或重命名文件后更新本索引。

如果某个问题重复出现，把原因和规避方式同步到 [`../pitfalls/README.md`](../pitfalls/README.md)。

## 文档模板

操作手册包含适用场景、前置条件、操作步骤、预期结果、验证方式、失败处理和回滚方式。
