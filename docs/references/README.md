# 上游参考资料

本目录存放 PRD、设计稿、平台规范和外部系统文档的快照，用于保留需求和约束的原始来源。

## 文件索引

| 文件 | 内容 | 什么时候读取 |
|---|---|---|
| [`ai-cool-2.0-requirements.md`](./ai-cool-2.0-requirements.md) | AI Cool 2.0 会议逐字稿摘录、范围判断与当前实现证据 | 回顾 AI Cool 2.0 原始需求时 |
| [`ai-cool-2.0-benchmark/README.md`](./ai-cool-2.0-benchmark/README.md) | 竞品、开源项目调研、AI Hot 专项结论与 Ego Lite 页面截图 | 需要外部产品参考时 |
| [`openapi.yaml`](./openapi.yaml) | 当前项目 OpenAPI 参考契约 | 修改接口或联调时 |
| `<主题>-prd.md` | 某项需求的原始 PRD | 开发对应需求时 |
| `<系统>-integration.md` | 外部系统接入资料 | 对接对应系统时 |
| `<规范名称>.md` | 平台或技术规范 | 涉及对应规范时 |

## 维护规则

### 命名规则

需求文档使用 `<主题>-prd.md`，外部接入文档使用 `<系统>-integration.md`，平台规范使用能够表达规范名称的英文文件名。

### 新增和更新规则

出现新的独立上游资料时新增文件；上游内容变化时同步原文件，不创建多个重复版本。新增、删除或重命名文件后更新本索引。

Cooper 来源的文档按下面「Cooper 镜像约定」带 frontmatter，交给 `cooper-requirement-doc-sync` skill 统一同步，不要手动复制粘贴正文。

这里是“事实来源”，不是“当前实现”；实现状态以 [`../overview/implementation-status.md`](../overview/implementation-status.md) 为准。

## 文档模板

每篇文档应包含标题、来源链接、来源类型、获取或同步时间、原始内容以及必要的附件说明。Cooper 镜像文档还应保存资源 ID 和最后同步时间。

## Cooper 镜像约定

从 Cooper（知识库 / 文档）镜像下来的参考文档，必须带下面的 frontmatter；`cooper-requirement-doc-sync` skill 据此识别镜像、开工前提醒“需求有没有更新”、并一键重拉。

```markdown
---
cooper_resource_id: "123456"
cooper_app_id: 4
source: <Cooper 链接>
title: <标题>
last_synced: "YYYY-MM-DD HH:MM"
---

<正文，markdown>
```

- 识别规则：只有以 `---` 起始、且开头 frontmatter 块里含 `cooper_resource_id` 的 `.md` 才算镜像。
- 新增镜像用 skill 的 `add`，不要手写 frontmatter 漏字段。
- 不要手改镜像正文；需要加工后的结论时，写到 [`../design/`](../design/) 或 [`../decisions/`](../decisions/)。
- 同步后关注正文增删；图片和附件链接重签通常是噪音。
