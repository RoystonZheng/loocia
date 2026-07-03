# 内网 AI 资讯精选服务(aihot 复刻)— 设计文档

- 日期:2026-07-03
- 负责人:@刘玥含
- 状态:待评审(brainstorming 产出)

## 1. 背景与目标

团队希望把 `aihot.virxact.com`(中文 AI 资讯精选服务,含每日精编日报 + 当前热点 + 分类检索)整套能力迁移到滴滴内网,做成一个**内网网页产品**:员工打开就能看 AI 日报、当前热点、按分类/关键词检索,并可选地通过 Skill / RSS / REST API 接入 Agent。

### 关键约束与既定事实(来自前期调研)

1. **源码不可得**:GitHub `KKKKhazix/khazix-skills/aihot` 只含一个 `SKILL.md`(调用手册,MIT),没有前后端源码。前端 bundle、后端服务全在对方服务器,未开源。因此这是**复刻/重建**,不是 lift-and-shift。
2. **API 契约完全公开**:对方发布了 `openapi.yaml`(v1.1.0),把 6 个只读端点 + 完整数据模型(`Item`/`DailyReport`/`HotTopic`/`DailyEntries`)、分类、过滤规则、cursor 翻页、weak ETag/304、限流、7 天窗口全部写明。**API 与前端可以 1:1 复刻**。
3. **核心"智能"未公开(产品壁垒)**:spec 明确剥离了五轴评分明细、`editorialJudgment` 编辑判断、事件聚类(`clusterId`/`duplicateOfId`/primary-secondary)、热度算法(多源计数 + 指数衰减)。**这些精选智能必须自建。**
4. **许可证红线**:`license: Reference Use`;`/about` 声明"聚合摘要,原文版权归各来源"。→ 学它的 API 设计/数据模型/交互 = 正当(参考用途);把它 curated 的数据搬进公司产品 = 版权灰区。**结论:内网直接吃对方 API 当正式产品的方案被否,复刻自建 + 自产数据是唯一正道。**
5. **可复用存量**:团队已有大量同类零件——`wechat-corpus`(公众号出料)、`wechat-push-agent`(跨机解耦 + 按 id 幂等去重)、`mkt-autodrive-news-agent` 的两层精选筛选口径(`mkt-report-focus-criteria`)、AI Guard Proxy(内网合规 LLM)、Melos 开发机。复刻可复用约 70%。

### 目标(本期"都做")

- 全部数据源:RSS(官方博客 + arxiv)+ 公众号 + X 精选账号
- 全部前端:接入引导页 + 精选列表 + 当前热点区 + 日报页 + 存档 + 详情页 + 搜索
- 全部智能:LLM 摘要/翻译、五轴评分、分类、精选判定、**事件聚类**、**热度算法**、每日主编日报
- 全部接入:REST API(1:1 复刻 openapi.yaml)+ SKILL.md + RSS 输出

### 非目标

- 不搬运对方 curated 数据(版权)。
- 不复刻对方 UI 像素级外观(用司内组件库自绘,规避版权 + 贴合内网视觉)。
- 不做站内登录态的 `/mp` 公众号爆文工作台(对方内部功能,超范围)。

## 2. 技术选型

- **后端**:Go(用 nuwa 脚手架起 HTTP 服务,符合团队 VSA/nuwa 主栈)
- **前端**:React + 司内组件库,详情页服务端渲染
- **存储**:PostgreSQL + `pg_trgm` GIN 索引(与对方同款,搜索直接对齐 spec)
- **LLM**:内网 AI Guard Proxy(合规)
- **缓存/网关**:nginx proxy_cache + weak ETag/304(对齐 spec 的 cron 低成本轮询)
- **部署**:先在 Melos 开发机(`root@10.190.12.242`)跑通 → 再上内网正式(nuwa/鲁班发布)
- **鉴权**:内网 SSO 网关 + 匿名只读(对内可读,不需 token 调用)
- **跨机中转**:采集侧(外网)→ staging(对象存储或 staging 表)→ 加工侧(内网),沿用 push agent 的跨机解耦模式

## 3. 架构总览

跨机解耦,三层加工:

```
采集层(外网侧机器)
  RSS(OpenAI/Anthropic/DeepMind/Meta/Mistral/arxiv) + 公众号(wechat-corpus 出料) + X 精选账号
  → 归一化 RawItem → 按 url hash 幂等 → 写中转
        │  对象存储 / staging 表
加工层(内网 AI Guard Proxy)7 道工序
  ①翻译+中文摘要 ②五轴评分→score/aiRelevance ③分类 ④精选判定 aiSelected
  ⑤事件聚类(primary/secondary + duplicateOfId) ⑥热度(多源计数+指数衰减) ⑦日报主编
        │
存储(内网 PostgreSQL + pg_trgm):items / clusters / dailies / sources
        │
API(内网,1:1 复刻 openapi.yaml):items · daily · daily/{date} · dailies · hot-topics · version · /items/{id}
  weak ETag+304 · cursor 翻页 · 限流 600r/min · items 7 天窗口
        │
前端(内网,React+司内组件库):/agent 接入引导 · 精选列表+热点区 · /daily 日报+存档 · /items/{id} 详情 · 搜索
        └─(可选都做)SKILL.md 指向内网 API · RSS 输出(daily.xml / feed)
```

## 4. 组件设计

### 4.1 采集层(外网侧)

- **Fetcher**:按源轮询(RSS 解析 / 公众号出料复用 / X 源),归一化为 `RawItem { sourceId, sourceName, sourceKind, url, title, publishedAt, rawContent }`。
- **幂等**:`id = hash(url)`,已存在则跳过(复用 push agent 去重)。
- **Handoff**:新 RawItem 写入中转(对象存储 / staging 表),内网侧消费。中转不可达时采集侧本地缓冲,下轮补投。
- **调度**:每 N 分钟一轮(可配)。

### 4.2 加工层(内网,LLM 管线)

按条目串行 7 道工序(可分阶段落库,支持断点续跑):

1. **翻译 + 中文摘要**:LLM 产出中文 `title`、`summary`;原标题不同则存 `title_en`;抓正文存 `body`(仅供详情页,不经 API 直出)。
2. **五轴评分**:LLM 按 `mkt-report-focus-criteria` 的口径打分 → `score`(0-100)、`aiRelevance`;`aiRelevance < 60` 判定相关性弱,从 `all` 池剔除。
3. **分类**:归入 `ai-models / ai-products / industry / paper / tip`。
4. **精选判定**:综合评分 + 编辑判断标 `aiSelected`;进入 `mode=selected` 主菜单。
5. **事件聚类**:对同事件多源报道聚簇。方法:embedding 相似度 + 时间窗聚类;完全重复标 `duplicateOfId`;簇内选 primary(最高分/最早报道),其余标 secondary/related。`selected` 只出簇 primary,避免同事件刷屏。
6. **热度**:按簇计算 `heat = 窗口内独立信源数 + 指数时间衰减`;记录 `sourceCount`、`sourceNames`、`latestAt`;定时重算(衰减)。
7. **日报主编**:每日定点,取窗口内 selected 条目 → LLM 生成 `lead(title + leadParagraph)` + 按分类分 `sections` + `flashes`,落 `dailies`。

失败处理:LLM 失败入队重试;`score` 允许暂空(下游判空,契约允许)。

### 4.3 存储层(内网 PostgreSQL)

- `items`:`id, title, title_en, url, permalink, source, source_kind, published_at, timeline_at, summary, body, category, score, ai_relevance, ai_selected, selected, cluster_id, duplicate_of_id, present`
- `clusters`:`id, primary_item_id, heat, source_count, latest_at`
- `dailies`:`date, generated_at, window_start, window_end, lead_title, lead_paragraph, sections(jsonb), flashes(jsonb)`
- `sources`:`id, name, kind, enabled`
- 索引:`pg_trgm` GIN on `title / title_en / summary / body`(对齐 spec 的 q 搜索 2-6ms);`published_at` 倒序索引(items 主排序契约)。

### 4.4 API 层(内网,复刻 openapi.yaml)

严格实现以下端点及其 spec 语义:

| 端点 | 说明 | 关键契约 |
|---|---|---|
| `GET /api/public/items` | 精选/全部/分类/时间/关键词 | 按 `publishedAt` 倒序;`mode` 默认 selected;隐式过滤(duplicateOfId/mp_hot/present=false);`since` 限 7 天;`q` 走 pg_trgm;cursor 不透明翻页;weak ETag + 304 |
| `GET /api/public/daily` | 最新日报 | 404 无日报 |
| `GET /api/public/daily/{date}` | 指定日日报 | 400 格式错 / 404 无 |
| `GET /api/public/dailies` | 日报存档索引 | take 1-180 |
| `GET /api/public/hot-topics` | 当前热点 | 按 heat 倒序;剥内部 clusterId/heat 数值,只出 sourceCount/sourceNames |
| `GET /api/public/version` | 版本信息 | 长缓存 + 304 |
| `GET /items/{id}` | 详情页(SSR) | 中文翻译 + 富文本 + 无墙;`noindex`;正文不经 JSON API 直出 |

- **限流**:单 IP 600 r/min(burst 40,超 429)。
- **缓存**:nginx proxy_cache 5 分钟;weak ETag + `If-None-Match` → 304 空 body。
- **错误**:400(参数越界)/ 404 / 429 / cursor 失效静默回首屏。

### 4.5 前端层(内网,React + 司内组件库)

- `/agent`:接入引导(Skill / RSS / REST API 三轨说明),镜像 aihot 的接入页语义。
- `/`(或 `/hot`):精选列表 + 顶部「当前热点」区(消费 hot-topics),卡片显示 score。
- `/daily`:最新日报 + 存档索引(消费 dailies / daily)。
- `/items/{id}`:详情页(SSR,中文翻译 + 富文本 + 无墙)。
- 搜索框 → `q` 参数。

### 4.6 接入层(可选,本期都做)

- **SKILL.md**:复刻对方 skill 结构,server 指向内网 API,触发词/输出格式按内网口径改。
- **RSS**:输出 `daily.xml` / `feed`,对齐 spec 的 timelineAt 语义。

## 5. 数据流与定时任务

- 采集:每 N 分钟(外网侧)。
- 加工:新 RawItem 触发(内网侧),7 道工序落库。
- 日报生成:每日定点(以 UTC 0 点为基准,展示转北京时间)。
- 热度重算:每 N 分钟(指数衰减)。

## 6. 错误处理

- 源不可达 → 跳过 + 记录 + 下轮重试(幂等不产生重复)。
- LLM 失败/限流 → 入队重试;部分字段(如 score)允许暂空,下游判空。
- API → 400/404/429 按 spec;cursor 失效/篡改/过期 → 静默回首屏。
- 中转不可达 → 采集侧本地缓冲,恢复后补投。

## 7. 测试策略

- **契约测试**:用 `openapi.yaml` 做 schema 校验,覆盖每个端点响应。
- **API 用例**:翻页(cursor)、ETag/304、限流、`since` 7 天边界、`q` 最小长度。
- **加工用例**:翻译/评分/分类用录制 fixture;**聚类用 golden 事件集**(已知同事件多源 → 期望聚成一簇 + 正确 primary);热度衰减用时间序列 fixture。
- **端到端**:灌原料 → 跑管线 → API 出预期日报 + 热点 + 列表。
- **前端**:关键页组件测试 + e2e。

## 8. 交付分期(即使"都做"也有序上线)

- **P1 — 骨架 + 主链路**:RSS 采集 → 存储 → items/daily API → 前端列表/日报/详情 → LLM 摘要/评分/分类/精选/日报生成。产出:能看日报和精选列表。
- **P2 — 智能 + 全源**:公众号 + X 源;事件聚类 + 热度 + hot-topics + 前端热点区。产出:当前热点可用,同事件不刷屏。
- **P3 — 接入 + 打磨**:SKILL.md + RSS 输出;weak ETag/304/nginx 缓存/限流;契约测试补全。

## 9. 待决 / 风险

- **公众号源合规**:内网使用公众号内容需确认版权/合规边界(沿用 wechat-corpus 既有结论)。
- **X 源采集通道**:X/推特在外网侧如何稳定取(RSS bridge / 既有源),需在采集层验证。
- **中转介质**:对象存储 vs staging 表,P1 起步用简单方案,按量再调。
- **聚类质量**:embedding + 时间窗的簇纯度需用 golden 集持续校准,是 P2 的主要不确定性。
- **scaffold**:进入实现前,按 `new-project-docs-scaffold` 约定跑 scaffolding-project-docs 铺标准 docs/,再写代码。
