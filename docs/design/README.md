# 模块设计

本目录维护各业务模块的长期设计，包括模块职责、数据模型、状态机、同步流程、组件和交互设计。

## 文件索引

| 文件 | 内容 | 什么时候读取 |
|---|---|---|
| [`ai-cool-2.0-product-design.md`](./ai-cool-2.0-product-design.md) | AI Cool 2.0 资讯、工具雷达、评测、工具箱与后续能力库的产品设计 | 回顾 AI Cool 2.0 总体方向时 |
| [`ai-cool-tool-discovery-prd.md`](./ai-cool-tool-discovery-prd.md) | AI Cool 工具发现、测评记录和团队工具列表的开发用 PRD | 开发本次工具发现闭环时 |
| [`ai-cool-tool-discovery-technical-solution.md`](./ai-cool-tool-discovery-technical-solution.md) | AI Cool 工具发现模块的轻量技术方案 | 评审本次工具发现实现方案时 |
| [`ai-cool-2.0-product-technical-plan/README.md`](./ai-cool-2.0-product-technical-plan/README.md) | AI Cool 2.0 历史技术方案和原型，部分口径已被工具发现 PRD 覆盖 | 追溯旧方案和原型材料时 |
| [`prototypes/ai-cool-2.0-tools-prototype.html`](./prototypes/ai-cool-2.0-tools-prototype.html) | 工具雷达与评测闭环的本地交互原型 | 对照早期工具页面交互时 |

## 维护规则

### 命名规则

模块设计使用 `<模块>.md`，配套表结构使用相同名称的 `<模块>.dbml`，前端专项设计使用 `<主题>-frontend.md`。

### 新增和更新规则

新增独立业务模块时新增文件；已有模块发生变化时更新原设计。新增、删除或重命名文件后更新本索引。

设计与代码冲突时，以代码和 [`../overview/implementation-status.md`](../overview/implementation-status.md) 为当前状态来源。

## 文档模板

模块设计包含背景、目标、非目标、模块职责、数据模型、核心流程、状态流转、异常边界、验证方式和相关文档。

## 边界

放：

- 后端：模块设计、DBML / 表结构、状态机、跨系统同步方案。
- 前端：设计 token / 主题配置、组件清单与复用约定、页面信息架构、视觉还原对照基线、关键交互流程。

不放：操作步骤和联调手册（那是 [`../runbooks/`](../runbooks/)）、上游原文与设计稿原件（那是 [`../references/`](../references/)）、接口字段和展示映射（那是 [`../api/`](../api/)）。
