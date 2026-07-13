# 评分体系重构 — Design

**Date:** 2026-07-13
**Status:** Approved (co-designed with user, all decisions locked)
**Goal:** Replace the vague 0-100 `score` with a 5-tier ordinal driven by an explicit
value model for the feed's mixed technical audience, and make 精选 quality-gated.

## Audience & value model (locked)

**受众**：做 AI 的技术人，既看工程落地、又看行业大势。

**「值得读」内核**：高分 = （工程可操作 **或** 行业重要）× 深度/证据（质量闸，横切）。
- 🔧 工程腿：有方法/代码/benchmark/踩坑，能照着用或学。
- 🌐 行业腿：头部玩家、格局变化、战略/投资信号、能力边界。
- 🚪 深度闸：讲清机制、有实测数据；浅的工程贴和浅的行业吹都过不了。
- **两条腿平权**（都能到顶档，看"打得多响 + 够不够深"，不预设天花板）。
- **不扣分**：旧闻/炒冷饭、不新颖、人尽皆知 —— 只要深且有用照样高分。
- **压低**：营销/宣发/通知（PR、发布会软文、活动/直播/招聘/预告）、标题党/无实质
  （标题吹正文空、观点无证据）、与 AI 弱相关（蹭热点、边缘沾边）。

## score：1-5 五档

Store integer **1-5** (5 = best); display **S/A/B/C/D**.

| 档 | 存 | 显 | 判据 |
|---|---|---|---|
| 重大突破/必读 | 5 | S | 一条腿打满 + 深度证据强（头部重大发布且讲透机制/有硬数据，或范式级工程实践带完整方法+benchmark） |
| 高价值 | 4 | A | 一条腿强 + 有实质深度（重要模型/产品更新且有分析，或扎实工程实践/研究） |
| 值得一看 | 3 | B | 相关且有信息量，但深度一般或影响有限（常规进展、能用不深，或行业动态但不关键） |
| 一般 | 2 | C | 边缘/增量小/偏宣发/浅尝 |
| 低 | 1 | D | 营销通稿 / 标题党 / 活动通知 / 与 AI 弱相关 |

元规则：看实质不看标题；旧但深仍可高分；缺新颖性不扣分。

**徽章分档配色**（`badge-score` 按档着色，暗/亮双色）：

| 档 | 显 | 主色 | 语义 |
|---|---|---|---|
| 5 | S | 琥珀金 `#f5a623` | 必读 |
| 4 | A | 站点蓝 `#2f6bff` | 高价值 |
| 3 | B | 青绿 `#17b26a` | 值得一看 |
| 2 | C | 中性灰 `#8a8f98` | 一般 |
| 1 | D | 淡灰 `#b4b8c0` | 低 |

徽章只在精选条目显示（当前 = B/A/S），但五档配色全定义好，方便后续调显示范围。用 `data-tier="S|A|B|C|D"` 属性 + CSS 变量着色。

## 精选决策（改）

- **精选 = `score ≥ 3`（B 档及以上）**，取代旧的 `selected OR relevance≥60`。这是**初始门槛**：上线后看真实分档分布，再定精选/feed 显示卡在哪档（可能收到 A 或放到 B）。
- LLM 的 `selected` 布尔**取消**（score-based 门槛替代了它，也解决了旧 selected 太保守的问题）。
- `relevance` **也降成 1-5 五档**（存 1-5，语义 = 与 AI/大模型主题的相关度：5 核心 AI、3 沾边、1 几乎无关），降级为"是不是 AI 主题"的过滤/分析信号，**不再当精选门槛**。展示上暂不露出（内部字段），有分布后再定要不要用它过滤。

## 热度

不变：`heat = 信源数 × 0.5^(距最新报道小时/24)`。

## Components

### 1. Prompt（`pipeline/enrichment.go`）
- 重写 `score` 字段为 1-5 五档 + 上面的判据 + 内核规则（工程/行业腿、深度闸、不扣旧闻、压低营销/标题党/弱相关）。
- `relevance` 改为整数 1-5：与 AI/大模型主题的相关度（5=核心 AI、3=沾边、1=几乎无关）。
- 删除 `selected` 字段（不再要模型判精选）。
- 保留 `reason_cn`、禁 ASCII 引号规则。

### 2. Enrichment struct（`pipeline/enrichment.go`）
- `Score int`（现语义 1-5）；`Relevance int`（现语义 1-5）；删除 `Selected bool`。

### 3. Selection（`pipeline/processor.go`）
- 删除 `selectionRelevanceFloor`；`selected := e.Score >= 3`。
- `ai_selected` 列保留但改为记录"强推"信号 `score >= 4`（A 档及以上），或置 nil。取 `score>=4`。

### 4. Schema + 迁移（`items/schema.sql` + 一次性 SQL）
- `score` 和 `ai_relevance` 两列 CHECK 都从 `BETWEEN 0 AND 100` → `BETWEEN 1 AND 5`。
- 迁移必须**先改数据后加约束**：老 `score`、老 `ai_relevance` 各自 0-100 归桶
  （`0-39→1, 40-59→2, 60-74→3, 75-89→4, 90-100→5`），再改 CHECK。
- 老 `selected` 按新门槛重算：`selected = (bucketed_score >= 3)`。
- 归桶是一次性近似（老分是旧 rubric 下的，不重跑 LLM）；新条目走新提示词得真档。

### 5. Frontend 展示（`web/src/`）
- `format.ts`：`scoreLabel(n)` → S/A/B/C/D。
- `ItemCard.tsx`：分数徽章显示字母 + `data-tier` 属性（仍只在精选条目显示）。
- CSS：`.badge-score[data-tier="S|A|B|C|D"]` 按上表分档着色（暗/亮双主题各定义）。
- 详情页 SSR（`detailpage/page.go`）：分数显示字母，徽章同款分档配色。
- relevance 不在前端露出（内部字段）。

### 6. Daily / Hot（无改动）
- `daily` 按 score 排序、`cluster.choosePrimary` 取高分——1-5 仍是"越高越好"，逻辑不变。

## Error handling
- 模型给了范围外的 score（如 0 或 6）→ `parseEnrichment` **钳制到 [1,5]**（`<1→1, >5→5`），不再当解析失败，避免因边界值丢条目。
- 这取代老的 `score>100→报错` 逻辑；旧测试 `TestParseEnrichmentRejectsOutOfRangeScore` 改为断言钳制结果（如 score=9→5）。

## Testing
- `enrichment_test`：score/relevance 钳制 1-5（越界值→边界）；prompt 含五档字样、含内核关键词（工程/行业/深度/营销）、不含 `selected` 字段要求。
- `processor_test`：score=3→selected=true，score=2→false；ai_selected=score>=4。
- `format.test`：scoreLabel 映射（1→D … 5→S）。
- `ItemCard.test`：精选项显字母 + 正确 `data-tier`、非精选不显。

## Deployment
- ALTER：先归桶老数据 + 重算 selected，再改 CHECK（否则约束失败）。
- Ship：server + pulse（都碰 items/prompt）；web bundle。
- 无需重跑历史富化（归桶近似即可）；新文自动走新档。

## Follow-up（上线后按分布定，不在本次实现）
- 精选/feed 显示门槛的最终档位（初始 score≥3，看真实分档分布再收/放）。
- relevance 是否用来做前端过滤 / 要不要露出。

## Out of scope
- 重跑全部历史条目的 LLM 打分（成本高；归桶近似够用）。
