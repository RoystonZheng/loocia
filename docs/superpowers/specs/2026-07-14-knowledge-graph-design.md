# 交互式词云图谱（横向聚类）— Design

**Date:** 2026-07-14
**Status:** Approved (co-designed with user)
**Goal:** 新增「图谱」主视图：经典填充式词云看「最近热在哪」，点词展开共现子图 + 相关资讯，回答「谁跟谁连着」，把散落的资讯横向聚起来。

用户已拍板的关键决策：
- **形态**：词云入口 → 点词开子图 + 资讯列表（brainstorm 选项 C）。
- **节点**：实体（公司/模型/产品/人）+ 话题关键词混合，LLM 抽取。
- **边**：同文共现，同一热点 cluster 内共现权重 ×2。不做带标签关系边（YAGNI，之后不够再加）。
- **落点**：侧栏新增一级视图「图谱」。
- **时间窗**：7 天 / 30 天 / 全部 可切换，默认 7 天。
- **渲染**：经典填充云朵观感，用 `d3-cloud` 算布局 + React 自渲 SVG（不引 ECharts，~35KB vs ~1.2MB）；子图 `d3-force`。两包均本地打进 Vite bundle，无 CDN。

---

## 数据层：`item_terms` 表

```sql
CREATE TABLE IF NOT EXISTS item_terms (
    item_id TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    term    TEXT NOT NULL,
    kind    TEXT NOT NULL CHECK (kind IN ('entity','topic')),
    PRIMARY KEY (item_id, term)
);
CREATE INDEX IF NOT EXISTS item_terms_term_idx ON item_terms (term);
```

- 加进 `items/schema.sql`（EnsureSchema 幂等建表，照 clusters 表的既有模式）。
- 归一化在抽取 prompt 内完成（见下），**不建别名映射表**。同一 item 内 entity/topic 撞词时 entity 优先（写入前去重）。
- 新 store：`server/internal/terms/store.go` — `ReplaceForItem(itemID, terms)`（delete+insert 事务）、词云/共现/相关资讯三个查询方法。

## 抽取层：照翻译模式抄

- `pipeline/terms.go`：`TermExtractor` 接口 + LLM 实现。输入 **title + summary**（不用全文——省 token、语义足够）。一次调用输出 JSON：`{"entities": [...], "topics": [...]}`，实体 ≤5、话题 ≤3。
- Prompt 归一化要求：实体用官方规范名（产品/模型保留英文原名，如 GPT-5.5、Claude；公司用通用中文名或官方英文名，全库一致）；话题词 2~6 个字的中文抽象词（如 推理模型、开源、具身智能）；输出只有 JSON。
- 模型：低档 client，`llm.NewTermsClientFromEnv()` 照 `AIHOT_TRANSLATE_MODEL` 的模式，env **`AIHOT_TERMS_MODEL`**，默认 `deepseek-v4-flash`。429 重试/退避复用现有 client。
- 挂载：`Processor.WithTermExtractor(t)`，在 ProcessBatch 内 **best-effort**——放主 upsert 之后（terms 是独立表，不阻塞 item 落库；失败只记日志不 return error）。解析失败/空结果同样跳过。
- 存量回填：`cmd/backfillterms`，照 `backfilltranslate` 抄（`-limit N`，跳过已有 terms 的 item）。977 条 × 低档模型 ≈ 1 小时（20 RPM 上限内），Melos 上 `nohup` 跑。

## API：两个公共端点（实时 SQL，不预计算）

handler 放 `publicapi/graph_handlers.go`，照 hot_handlers 范式。

**`GET /api/public/graph/cloud?window=7d|30d|all`** → 词云数据

```json
{ "terms": [ { "term": "OpenAI", "kind": "entity", "count": 42 } ] }
```

- SQL：`item_terms JOIN items` 按 `published_at >= now()-window` 过滤（`all` 不过滤），`GROUP BY term, kind ORDER BY count DESC LIMIT 80`。
- window 参数解析放 `params.go`，非法值按默认 7d 处理（公共 API 宽容风格，与现有 mode 参数一致）。

**`GET /api/public/graph/term/{term}?window=…`** → 子图 + 相关资讯

```json
{
  "term": "OpenAI", "kind": "entity", "count": 42,
  "neighbors": [ { "term": "推理模型", "kind": "topic", "weight": 18 } ],
  "items": [ { "…": "与 items feed 相同的公共字段" } ]
}
```

- neighbors：`item_terms` 自 join（同 item_id、term ≠ 焦点词），基础权重 = 共现 item 数；join items 后同 `cluster_id`（非空）的共现对权重 ×2；`ORDER BY weight DESC LIMIT 20`。
- items：含焦点词的 items 按 `published_at DESC LIMIT 20`，复用 publicitem 的公共字段序列化。
- 焦点词不存在 / 窗口内无数据 → 200 + 空数组（不是 404，前端好处理）。

## 前端：`GraphView.tsx`

- 侧栏（`Sidebar.tsx`）加一项「图谱」；路由/视图切换照现有精选/全部/日报模式。
- 新增 `web/src/api/graph.ts`（两个 fetch 封装）+ `web/src/components/GraphView.tsx`。
- **词云（上半屏）**：`d3-cloud` 布局，`rotate=0` 全横排（竖排中文难读），字号按 `√count` 线性映射到 14~48px，React 渲 `<svg>` + `<text>`。entity/topic 两色（现有主题 token，如 `--accent` 与一个次色，亮暗主题都适配）。hover 高亮，点击 → 展开下方面板并请求 term 端点。
- 顶部控件：7 天 / 30 天 / 全部 切换（默认 7 天），切换重取 cloud + 已选词的 term 数据。
- **详情面板（下半屏，选词后出现）**：左 = `d3-force` 静态布局子图（焦点词居中放大，邻居节点 r ∝ weight、边宽 ∝ weight；布局跑固定迭代次数后渲染，v1 不做拖拽动画），点邻居节点 = 切换焦点词；右 = 相关资讯列表（标题行 + 来源 + 日期，链到 `/items/{id}` 详情页）。
- 移动端（600px 断点，沿用现有断点体系）：词云 SVG 按 viewBox 缩放，面板左右改上下堆叠。
- 空态：窗口内无 terms → 提示「该时间段暂无数据」；接口失败同现有视图的容错风格（不炸整页）。

新 npm 依赖：`d3-cloud`、`d3-force`（+ `@types/d3-cloud`、`@types/d3-force`）。

## 错误处理

- 抽取失败/解析失败：日志 + 跳过，item 主流程不受影响；下次不自动重试（terms 缺失可由 backfillterms 补，YAGNI 不做重试队列）。
- LLM 429：client 既有退避。
- 前端两个接口任一失败：图谱页显示错误空态，其他视图不受影响。

## 测试

- **Go**：`pipeline/terms_test.go` 假 LLM 测 prompt 输出解析（合法 JSON / 带代码块包裹 / 超限截断 / 空结果）；`terms/store_test.go` 走 `AIHOT_TEST_DATABASE_URL`（无则 skip）测 ReplaceForItem 幂等 + 三个查询（含 cluster 加权）；`publicapi/graph_handlers_test.go` 照 e2e 范式测两个端点 + window 参数。
- **前端**：vitest + `fireEvent`（不用 user-event）。jsdom 无 canvas，**mock d3-cloud 布局函数**返回固定坐标，测：渲染 top 词、点击词展开面板并带对 term、切换窗口重新请求带对 window 参数、空态。
- 全量：`server/` 下 `GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test`（-p 1）；`web/` 下 `npx vitest run` + `npm run build`。

## 发布（Melos）

1. 交叉编译（`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`）`server` + `backfillterms` → scp `<name>.new` → `chmod +x` → `mv -f` 原子替换 → `supervisorctl restart aihot-server`。
2. `rsync web/dist/` → `/var/www/aihot/`。
3. `nohup` 跑 backfillterms 回填存量（环境从 `aihot-cron.sh` 同款 export），本地 poll 进度。
4. 增量抽取挂在 pulse 已有 ProcessBatch 里，**无新 cron 单元**；`aihot-cron.sh` 若需自定义模型加一行 `AIHOT_TERMS_MODEL` export（默认值可不加）。

## 明确不做（YAGNI）

- 带标签关系边（发布/收购…）、别名映射表、embedding/语义相似边、预计算快照表、词云拖拽/缩放动画、抽取重试队列。
