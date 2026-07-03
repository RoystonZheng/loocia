# References

本目录沉淀上游资料的事实来源快照：PRD、外部系统接入文档、平台规范等。开发全新功能时，先从这里确认需求和约束的原始出处。

放：上游 PRD、设计稿 / 原型（Figma / 蓝湖等）、外部接入文档、平台/规范文档的快照和摘要。
不放：已经加工过的技术决策（那是 [`../decisions/`](../decisions/)）、模块设计（那是 [`../design/`](../design/)）。

## 维护规则

- 新需求来自 PRD 或外部系统时，先把原始资料沉淀到这里，再写设计和决策。
- 设计稿 / 原型（Figma / 蓝湖等）不在 Cooper 时，沉淀关键截图 + 链接 + 获取时间到这里（Cooper 镜像约定不适用），作为前端视觉还原基线的来源；视觉还原对照本身放 [`../design/`](../design/)。
- 保留来源链接和获取时间，注明是快照（可能与上游最新版本不一致）。
- Cooper 来源的文档按下面「Cooper 镜像约定」带 frontmatter，交给 `cooper-requirement-doc-sync` skill 统一同步，不要手动复制粘贴正文。
- 这里是“事实来源”，不是“当前实现”；实现状态以 [`../overview/implementation-status.md`](../overview/implementation-status.md) 为准。

## Cooper 镜像约定

从 Cooper（知识库 / 文档）镜像下来的参考文档，**必须带下面的 frontmatter**；`cooper-requirement-doc-sync` skill 据此识别镜像、开工前提醒“需求有没有更新”、并一键重拉。

```markdown
---
cooper_resource_id: "123456"   # Cooper 资源 ID，必填，加引号
cooper_app_id: 4               # 知识库=4，Cooper 文档=2
source: <Cooper 链接>
title: <标题>
last_synced: "YYYY-MM-DD HH:MM"
---

<正文，markdown>
```

- **识别规则**：只有「以 `---` 起始、且开头 frontmatter 块里含 `cooper_resource_id`」的 `.md` 才算镜像；仅在正文 / 代码块里出现该字样的（比如本 README 上面的格式示例）不算，不会被同步。
- **新增镜像用 skill 的 `add`**（给 Cooper 链接或 resourceId），别手写 frontmatter 漏字段。
- **不要手改镜像正文**：同步时 skill 会重拉正文、保留 frontmatter、刷新 `last_synced`，手改的正文会被下次同步覆盖。需要加工后的结论时，写到 [`../design/`](../design/) 或 [`../decisions/`](../decisions/)。
- **同步后看「变了什么」**：skill 用词级 diff 显示需求增删；图片 / 附件链接每次重拉都会重签、属噪音，关注正文增删即可。

> frontmatter 字段和识别规则以 `cooper-requirement-doc-sync` skill 的「镜像约定」为准；该 skill 调整时同步更新本节。
