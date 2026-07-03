# 项目文档（docs）入口与维护规范

本文件是项目文档（`docs/`）的统一入口和总纲：规定默认技术选型、文档怎么组织、何时更新、冲突时听谁的，并索引各目录。AI 和新人进入项目先读它。**各类文档的详细写作格式和约定，见对应子目录的 `README.md`。**

本规范与具体业务无关，可原样放进任意项目的 `docs/`；项目专属的文档索引由 AI 在维护过程中逐步补充。**本模板前后端通用**：后端口径与前端口径在同一份文档里并列，按项目实际取用。

## 一、默认技术选型

除非项目另有决策，按以下默认技术选型；如需偏离，在 [`decisions/`](./decisions/) 记一条 ADR 说明原因，并在 [`overview/technical-stack.md`](./overview/technical-stack.md) 写明实际选型。

| 维度 | 默认 |
|---|---|
| 前端语言 | TypeScript |
| 前端框架 | React |
| 前端组件库 | Ant Design |
| 前端样式方案 | 主题 token / CSS-in-JS（`ConfigProvider` + design token），避免散落硬编码 |
| 后端语言 | Go |
| 后端框架 | Nuwa |
| 数据库 | MySQL |

> 前端的状态管理、数据请求 / 缓存、构建工具、测试框架按项目实际选型，统一在 [`overview/technical-stack.md`](./overview/technical-stack.md) 写明。

## 二、AI 工作协议

1. 先判断任务场景，按「场景入口」读取最小必要文档；不要一上来就从代码或 PRD / 设计稿改。
2. 读完文档后，先用「读后确认格式」回复，再动代码或文档。
3. 任何改变接口、数据模型、业务规则、**组件契约 / 公共组件、设计 token / 主题、路由、交互规则**、部署 / 联调方式的改动，都要按「文档更新规则」同步更新对应文档。
4. 文档与代码冲突时，以代码和 [`overview/implementation-status.md`](./overview/implementation-status.md) 为当前状态来源，并在回复中指出冲突。
5. 不同类型的内容写进不同目录，不要混写（见「目录职责」）；具体写作格式遵循该目录 `README.md`。
6. 新增文档后，更新所在目录 `README.md` 的索引和本文件的「核心文档索引」。

## 三、读后确认格式

```text
任务场景：接手项目 / 继续未完成任务 / 新功能开发 / 联调 / 视觉还原 / 排障 / 历史方案回顾
已读文档：列出实际读取的 docs 路径
当前理解：用 3-5 条说明项目状态、相关模块（前端含相关页面 / 组件）、关键约束（前端含设计稿 / 接口 / 交互态）
待确认问题：没有就写“无”
执行计划：列出将修改的代码 / 文档范围和验证方式（前端含交互态 / 响应式 / 截图）
```

## 四、目录结构

```text
docs/
├── README.md                 # 本文件：入口与总纲（索引 + 更新/维护规则）
├── overview/                 # 项目总览：做什么、怎么搭、做到哪
├── references/               # 上游资料快照：需求与约束的事实来源（含设计稿 / 原型）
├── decisions/                # 关键技术与业务决策（ADR）
├── design/                   # 长期设计：后端 模型/表结构/状态机 + 前端 设计token/组件/视觉还原/交互
├── api/                      # 接口契约：后端契约 + 前端消费契约（字段↔展示、错误码↔错误态）
├── runbooks/                 # 操作手册：开发、联调、排障、运维、（前端）构建与视觉验证
├── pitfalls/                 # 已验证的踩坑与规避
├── handoff/                  # 阶段性接手包索引
└── superpowers/              # 历史 specs/plans 归档（非当前事实源）
```

每个目录的 `README.md` 写明该目录的详细约定、文档格式和文件索引。

## 五、目录职责

| 目录 | 作用 | 放什么 | 不放什么 |
|---|---|---|---|
| [`overview/`](./overview/) | 项目总览，新人与 AI 的第一入口 | 项目目的、业务模型 / 页面地图、架构、技术栈、实现状态 | 接口字段细节、临时草稿 |
| [`references/`](./references/) | 沉淀上游事实来源 | PRD、设计稿 / 原型、外部接入文档、平台规范的快照与摘要 | 已加工的技术决策 |
| [`decisions/`](./decisions/) | 记录关键取舍与缘由 | 决策背景、最终方案、后果、被否选项 | 大段 PRD、接口明细 |
| [`design/`](./design/) | 模块的长期设计（前后端） | 后端：模块设计、表结构 / DBML、状态机、同步方案；前端：设计 token / 主题、组件清单与复用约定、视觉还原基线、关键交互流程 | 操作步骤、联调手册 |
| [`api/`](./api/) | 接口契约（生产方 + 消费方） | 后端：请求 / 响应、错误码、分页、鉴权、字段规则；前端：字段↔展示映射、错误码↔错误态、各态数据条件、mock 约定 | 业务背景长文 |
| [`runbooks/`](./runbooks/) | “怎么操作”的手册 | 启动、配置、联调、排障、运维步骤；前端：构建、lint、组件测试、浏览器视觉验证 | 需求讨论、方案脑暴 |
| [`pitfalls/`](./pitfalls/) | 已验证的坑与规避 | 症状、原因、修复 / 规避方式 | 未经验证的猜测 |
| [`handoff/`](./handoff/) | 阶段性交接索引 | 某版本 / 模块的关键信息串联 | 长期通用设计 |
| [`superpowers/`](./superpowers/) | 历史过程产物归档 | 自动生成的 specs 与 plans | 需第一时间阅读的权威文档 |

## 六、文档更新规则

| 改动类型 | 必须同步更新 |
|---|---|
| 新增或完成模块 / 页面 | [`overview/implementation-status.md`](./overview/implementation-status.md)，必要时更新 [`overview/architecture.md`](./overview/architecture.md) |
| 改业务实体、状态机、表结构 | 相关 [`design/`](./design/)，必要时新增/更新 [`decisions/`](./decisions/) |
| 改组件契约 / 公共组件 / 组件拆分 | 相关 [`design/`](./design/)，必要时记 [`decisions/`](./decisions/) ADR |
| 改设计 token / 主题 / 全局样式 | [`design/`](./design/) 下设计 token / 主题，必要时记 [`decisions/`](./decisions/) ADR |
| 改接口、错误码、字段、分页、鉴权 | [`api/`](./api/) 下契约文件 |
| 改字段展示映射 / 错误态 / 交互态 | [`api/`](./api/)（展示映射、各态数据条件）+ 相关 [`design/`](./design/)（交互流程） |
| 改路由 / 页面结构 | 相关 [`design/`](./design/)，必要时 [`overview/architecture.md`](./overview/architecture.md) |
| 改启动、部署、联调、排障、构建方式 | 相关 [`runbooks/`](./runbooks/) |
| 改技术选型（语言/框架/组件库/存储等） | [`overview/technical-stack.md`](./overview/technical-stack.md)，并在 [`decisions/`](./decisions/) 记 ADR |
| 发现重复踩坑 | [`pitfalls/`](./pitfalls/) |
| 新需求来自 PRD / 设计稿 / 外部系统 | 先沉淀到 [`references/`](./references/)，再写设计和决策 |
| 阶段性交接 | [`handoff/`](./handoff/) |

## 七、文档权威顺序

同一问题多个文档说法不一致时，按此顺序判断：

1. 当前代码（含数据库迁移、路由 / 组件实现）。
2. [`overview/implementation-status.md`](./overview/implementation-status.md)。
3. 相关 [`design/`](./design/) 和 [`api/`](./api/)。
4. 相关 [`decisions/`](./decisions/)。
5. [`references/`](./references/) 中的上游资料（PRD / 设计稿）。
6. [`superpowers/`](./superpowers/) 中的历史 specs/plans。

## 八、场景入口

| 场景 | 必读顺序 |
|---|---|
| 接手项目 | `overview/product.md` → `domain-model.md` → `architecture.md` → `implementation-status.md` |
| 继续未完成任务 | `overview/implementation-status.md` → 相关 `design/` → 相关 `api/` → 相关 `decisions/` |
| 开发全新功能 | `references/`（PRD / 设计稿）→ `decisions/` → `overview/domain-model.md` → 相关 `design/` → `api/` → `runbooks/` |
| 修改接口 / 联调 | `api/` 契约 → `overview/implementation-status.md` |
| 视觉还原 / 改样式主题 | `design/`（设计 token / 视觉还原基线）→ 相关组件 |
| 排障 / 运维 | `runbooks/` → `pitfalls/` → `overview/technical-stack.md` |
| 回顾历史方案 | `superpowers/` → 再回到 `implementation-status.md` 和当前设计确认 |

## 九、禁止事项

- 不要只读 `superpowers/` 就开始实现；它是历史产物，不是当前事实源。
- 不要只根据 PRD / 设计稿改代码；先检查已有 ADR、设计、接口和实现状态。
- 不要把上游原文、技术决策、接口契约、操作手册混写到同一文档。
- 不要改了接口、数据模型、组件契约、设计 token 却漏改 docs。
- 不要静默处理文档冲突；发现冲突要在回复中说明。

## 十、核心文档索引（AI 维护）

AI 在新增/调整文档时维护本节。`overview/` 五篇是固定起点；其余目录的核心文档随项目推进在此登记，详见各目录 `README.md`。

| 文件 | 说明 |
|---|---|
| [`overview/product.md`](./overview/product.md) | 项目目的、用户、核心能力 |
| [`overview/domain-model.md`](./overview/domain-model.md) | 业务实体、关系、状态机、关键规则；前端：页面 / 路由地图、核心交互流程 |
| [`overview/architecture.md`](./overview/architecture.md) | 系统模块、外部依赖、交互边界；前端：路由、组件分层 |
| [`overview/technical-stack.md`](./overview/technical-stack.md) | 语言、框架、存储、测试、部署；前端：React / 组件库 / 状态管理 / 构建 / 测试 |
| [`overview/implementation-status.md`](./overview/implementation-status.md) | 已实现、部分实现、待办和风险快照 |
