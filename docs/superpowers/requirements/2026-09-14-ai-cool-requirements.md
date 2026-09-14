# AI Cool 信息源扩展升级

## Clarified Requirements

本次开发范围限定为 Cooper 文档《AI Cool介绍》的第四部分“信息源扩展方案”，目标是在现有 AI Cool 信息流、入库、评分、精选和前端 Feed 链路上扩展信息来源覆盖面，提高高质量 AI 信息源的召回和自动化筛选质量。本次不包含第五部分“工具发现与沉淀方案”、不建设独立工具库、不新增管理后台页面、不改造六大 LLM 输出结构。

系统必须保留现有 RSS/公众号抓取、raw_items 入库、LLM enrichment、selected 判定、/api/public/items 输出和前端 Feed 展示的主链路。新增来源需要按配置驱动接入，支持 RSS/Atom、网页直采 HTML、公众号 MP、AIHOT 补漏四类来源，并把来源性质与采集方式解耦：source_kind 表示采集方式，source_role 表示内容角色，最小枚举为 official、professional、discovery。

第一阶段信息源名单以文档第四部分为准：官方源包括 Anthropic、Google DeepMind、Cloudflare AI、NVIDIA AI；专业源包括人人都是产品经理和极客公园；社区/发现源包括 Reddit LocalLLaMA 与 Reddit MachineLearning。当前验证结论需要进入实现约束：Anthropic News 为 HTML 直采候选；DeepMind、Cloudflare AI、NVIDIA Generative AI、woshipm、LocalLLaMA 当前 RSS 可达；Reddit MachineLearning RSS 在验证时返回 429，默认不开启或仅在部署环境验证通过后开启。公众号源当前仅人工/半自动维护，需支持按官方源角色注入，但不能假设已经具备公众号自动抓取能力。

AIHOT 作为二手线索补漏源接入，接口基准为 https://aihot.news/api/v1/items 等 v1 公共接口；AIHOT 数据不能替代原文链接，不能把 AIHOT 的热度/评分作为站内精选依据，不能向前端暴露 AIHOT 内部分数或全文能力。AIHOT item 有原文链接时以原文链接作为 ArticleURL/去重依据；缺少原文链接的二手线索只允许低比例进入候选池，需通过配置上限保护。

后端需要扩展 source 配置模型，至少包含 id、name、url、adapter/source_kind、source_role、enabled、fetch_limit、timeout、rate_limit、secondary_cap 等可配置项；新增 raw_items.source_role 持久化字段，并在 RawItem 领域模型中传递。去重仍以 canonical URL 为主，同一原文跨 RSS/AIHOT/网页直采出现时合并为同一 raw item，不应重复展示。

评分提示词和 enrichment 输入需要显式包含 source name、source kind、source role、标题、摘要/正文片段、URL 和发布时间。精选逻辑需改为双门槛：只有 relevance >= 4 且 score >= 3 的新内容可以入选 selected；AIHOT 只能提供候选，不得绕过 LLM 评分。六个模型输出字段和前端既有展示字段保持兼容。

前端只做信息流侧的来源感知展示，不新增独立页面。Feed 的来源筛选需要支持“全部来源、RSS/Atom、网页直采、公众号、AIHOT补漏”等分组；卡片可以继续展示 source 文案，其中 AIHOT 二手线索应体现“AIHOT · 二手线索”一类来源名，但不展示 source_role 内部枚举。/api/public/items 需要继续兼容既有消费者，并允许 source_kind 枚举扩展到 rss、html、mp、aihot。

验收时必须覆盖闭环用例：新增各类 source 配置可被 pulse 加载；RSS/HTML/AIHOT adapter 能分别产出 raw item；source_role 能从配置写入 raw_items；canonical URL 去重能处理同源跨渠道重复；enrichment 提示词包含来源上下文；selected 双门槛生效；AIHOT 原文链接优先和二手线索比例上限生效；前端来源筛选和未知枚举兼容；外部源失败、429、超时或格式漂移不会中断其他来源采集；数据库迁移可在空库和已有 raw_items 表上执行。

## Sources

- External: `https://cooper.didichuxing.com/didocs/2209600482571` at `read:2026-09-14T00:00:00+08:00` — Cooper 文档第四部分定义信息源扩展方案与第一阶段来源方向
