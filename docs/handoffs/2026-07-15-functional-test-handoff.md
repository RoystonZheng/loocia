# Handoff：aihot 功能完整性 + 健康度压测

**给：** 测试 agent
**日期：** 2026-07-15
**目标：** 对内网 aihot（AI 资讯站）做一次**完整功能验证 + 数据管道健康度审计 + 鲁棒性/边界测试**，产出一份 pass/fail 报告。重点不是常规回归，而是**发现"悄无声息坏掉"的问题**——功能没报错、页面能打开，但内容陈旧/翻译是垃圾/某条管道断了没人知道。

---

## 0. 为什么要这次测试（背景，必读）

aihot 最近连续踩了几类**静默失败**，测试要专门盯这些模式：

1. **数据管道断更没告警**：公众号内容冻在某天不再更新（跨机语料同步/导入的某一环停了，站点照常能开，但没有新内容）。
2. **翻译存了垃圾**：英文源翻译，模型拒翻或原样回英文，best-effort 只判"调用报错"没判输出内容，垃圾照存。
3. **正文不全**：RSS 只存 feed 摘要（一句话），详情页"原文"残缺。
4. **配置/凭证类**：LLM key 取值被 shell 转义 bug 破坏 → 401；服务照常起但翻译全挂。
5. **移动端**：布局在窄屏错位。

**结论**：功能"能用"≠"健康"。这次测试要同时覆盖【功能可用】+【数据新鲜/完整/质量】+【边界/异常】。

---

## 1. 被测系统

- **线上入口**：http://10.190.12.242:8899/ （nginx 前端 SPA + 反代后端）。
- **后端**：Go（nuwa 框架），Melos 上 supervisor 管理，进程 `aihot-server` 监听 `:8991`。仓库 `/Users/didi/aihot-internal/server`。
- **前端**：React + TS（Vite），仓库 `/Users/didi/aihot-internal/web`，构建产物部署在 Melos `/var/www/aihot/`。
- **数据库**：PostgreSQL 17，Melos 本机，**生产库 `postgres://aihot:aihot@localhost:5432/aihot`**（测试库 `aihot_test`）。
- **两类内容**：`source_kind='rss'`（英文源，需翻译成中文存 `body_cn`）、`source_kind='mp'`（公众号，本就中文，不翻译）。
- **管道**：RSS/公众号 → raw_items →（pulse 富集 LLM）→ items 表。cron：importmp :25 / pulse :30 / gendaily :10 / hotpass :20。

### 访问方式
- SSH：`ssh -o ClearAllForwardings=yes melos`（免密已配，root@10.190.12.242）。
- DB：`ssh melos 'psql "postgres://aihot:aihot@localhost:5432/aihot" -c "..."'`。
- API：`ssh melos 'curl -s "http://127.0.0.1:8991/api/public/..."'`，或走线上 `http://10.190.12.242:8899/api/public/...`。
- 日志：Melos `/root/aihot/log/{server,pulse,importmp,gendaily,hotpass}.log`。

---

## 2. 完整 API / 页面清单（功能覆盖面）

后端挂载点（`server/common/server/httpserv/httpserv.go`）：

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/public/version` | GET | 版本号 |
| `/healthz` | GET | 健康探针（依赖 DB ping） |
| `/api/public/items` | GET | **信息流主接口**：mode/category/source_kind/q/take/cursor/since |
| `/items/{id}` | GET | SSR 详情页（HTML）；`?format=md` 导出 markdown |
| `/items/{id}/retranslate` | POST | 失败重译（auto-std），返回 `{"ok":bool}` |
| `/api/public/daily` | GET | 最新日报 |
| `/api/public/daily/{date}` | GET | 指定日期日报 |
| `/api/public/dailies` | GET | 日报列表 |
| `/api/public/hot-topics` | GET | 热点聚类 |
| `/api/public/graph/cloud` | GET | 词云 |
| `/api/public/graph/term/{term}` | GET | 某词的关联图 |

前端视图（`web/src/App.tsx` 的 View）：`精选(selected)` / `全部(all)` / `日报(daily)` / `图谱(graph)`。信息流内含：**来源切换**（全部来源/公众号/英文源）、**分类 chips**（全部+5类）、**搜索框**、**加载更多**。

---

## 3. 测试范围（分四块，逐块出结论）

### A. 功能可用性（每个端点 + 每个 UI 控件都要点到）

**信息流 `/api/public/items`**——把参数矩阵跑全：
- `mode`：无参（默认=selected）、`mode=all`、`mode=selected`。
- `source_kind`：无、`mp`、`rss`、非法值(`=email`→**期望 400**)。
- `category`：无、5 个合法值(`ai-models/ai-products/industry/paper/tip`)、非法值(→**400**)。
- `q`：空、1 字符(应被忽略、仍套 7 天窗口)、≥2 字符(全时段搜索)、>200 字符(截断到 200)。
- `take`：默认(50)、1、100、0(→**400**)、101(→**400**)、`abc`(→**400**)。
- `cursor`：翻页——第一页拿 `nextCursor` → 带 cursor 拿第二页，**验证无重复、无跳漏、顺序按 published_at DESC**；乱码 cursor 应被忽略当第一页(不报错)。
- `since`：合法 RFC3339、未来时间(→**400**)、非法格式(→**400**)、早于 7 天(浏览时应 clamp、搜索/公众号时不 clamp)。
- **关键交叉**：`mode=all&source_kind=mp` 应返回**历史公众号**（不受 7 天窗口限制，能翻到数月前）；`source_kind=rss` 只出英文源且新鲜。

**详情页 `/items/{id}`**：
- 正常 rss（有译文）：出中文正文、有"← 返回"、有"看英文原文"切换、有"本文由 AI（<model>）翻译"、无重试按钮。
- rss 无译文：出英文原文、**有"翻译失败·点此重试翻译"按钮**、无译文声明。
- mp：中文正文、有返回键、**无**重试按钮、**无**译文声明。
- `?format=md`：返回 `text/markdown`，含 `## 原文（AI 翻译 · <model>）` 或原文标题。
- 404 场景：不存在的 id、`present=false`（撤下）、`duplicate_of_id` 非空（完全重复）→ 都应 404。

**重译端点 `POST /items/{id}/retranslate`**：
- rss 且有 body → `{"ok":true}` 且 DB `body_cn`+`body_cn_model` 被更新（model 应是 `auto-std`）。
- 非 rss / 无 body → **400**。
- 不存在 id → 404。
- 若某条内容确实翻不好（纯链接/极短）→ `{"ok":false}`（合理，不是 bug）。

**日报/热点/图谱/健康/版本**：各 GET 至少返回 200 + 结构合法 JSON；日报按日期取、列表非空；图谱 term 页对真实词返回关联。

**前端 UI**（用线上站点，真点）：
- 四个视图切换正常；来源切换 3 档都能刷新列表且 URL 带对参数；分类 chips 叠加来源筛选生效；搜索重置列表；加载更多追加不重复。
- **移动端**：窄屏（≤600px feed / ≤720px 详情）侧边栏收成顶栏，不横向溢出，卡片不错位。
- 明暗主题切换（light/dark/system）在 feed 和详情页都正确。

### B. 数据管道健康度（**本次重点**——静默失败在这里）

用 `docs/handoffs/` 附带的健康探针 SQL（见 §5）逐项查，任何一项异常都要标红：

1. **新鲜度**：
   - mp `max(published_at)` 和 `max(created_at)` 距今多久？（正常应有近 1-2 天内容；若冻结在某天→管道断，按 [[aihot-mp-ingestion-chain]] 排查）。
   - rss `max(published_at)` 是否接近今天？（英文源应每天有新的）。
2. **导入链在跑**：`importmp`/`pulse` cron 是否存在且近期有日志；`/root/wechat-corpus/` 语料是否新鲜；aihot 读的 `AIHOT_MP_CORPUS_DIR` 指向的目录是否新鲜。
3. **翻译完整性**：`source_kind='rss' AND body 非空 AND body_cn IS NULL` 的条数（=未翻/翻译失败）。占比高→报警。
4. **翻译质量（存量扫描）**：body_cn 里有没有**拒翻**（含"请提供/我会按/抱歉/无法翻译/作为一个/as an ai/please provide"）或**回英文**（去标签后 CJK 占比<10% 且长度>20）的漏网之鱼。
5. **正文完整性**：`source_kind='rss'` 里 body 去标签后 <500 字的占比（RSS 应大多已抓全文；过高→全文抓取没生效）。
6. **富集完整性**：present 的 items 里 `title` 为空 / `summary` 为空 / `category` 为空 / `score` 为空（1-5）/ `score` 越界(<1 或 >5) 的条数。
7. **评分分布**：score 1-5 各档条数是否合理（不应全挤一档）；精选(selected)=score≥3 是否自洽。
8. **去重/聚类**：`duplicate_of_id` 指向的目标是否 present；cluster_primary 逻辑是否让每簇只出一条。
9. **模型标记**：`body_cn_model` 分布（deepseek-v4-flash 为主，auto-std 为重试，null 为老数据）——有没有异常模型名。

### C. 鲁棒性 / 边界 / 异常

- 所有"→400"的非法参数**必须**返回 400 且不是 500/panic。
- 超长/畸形输入：`q` 塞 emoji/SQL 片段（`'; DROP`…）→ 不应报错、不应注入（用参数化查询，验证仍安全返回）。
- 不存在的日报日期、不存在的 term → 优雅空结果或 404，不 500。
- 重译端点在**无 LLM key** 时应返回 503（不 panic）；有 key 时正常。
- 并发/轻量压测（**这是"压测"部分**）：对 `/api/public/items` 和 `/items/{id}` 各并发 50~100 请求，观察：无 5xx、无明显延迟劣化、DB 连接池不耗尽、keyset 分页在并发下不串页。（工具：`hey`/`ab`/`wrk` 或 `xargs -P` + curl。**只读接口可压；重译端点会吃 LLM 额度，不要压。**）

### D. 一致性 / 契约

- API 返回字段与前端 `web/src/api/*.ts` 的 TS 类型对齐（字段名、可选性）。
- SSR 详情页与 API 对同一 id 的核心字段一致（标题、来源、是否精选）。
- markdown 导出内容与页面正文一致。

---

## 4. 已知失败模式清单（**逐条确认已修复/不复现**）

| # | 失败模式 | 验证点 |
|---|---|---|
| 1 | 公众号内容冻结不更新 | §B.1/B.2：mp 新鲜度 + importmp/pulse cron 在跑 + 语料目录新鲜 |
| 2 | 翻译存拒翻/回英文垃圾 | §B.4：存量扫描零漏网；新翻译经 `looksUntranslated` 校验 |
| 3 | RSS 正文只有一句摘要 | §B.5：短 body 占比低；抽查详情页出全文 |
| 4 | LLM key 取值被破坏→401 | §C：重译端点实测能成功出中文（证明线上 key 有效） |
| 5 | 移动端错位 | §A 前端移动端断点 |
| 6 | 详情页缺返回键/重试/模型声明 | §A 详情页三态 |

---

## 5. 健康探针 SQL（可直接跑，产出一张体检表）

> 在 Melos 上 `psql "postgres://aihot:aihot@localhost:5432/aihot" -c "<下面每条>"`。**只读，勿改数据。**

```sql
-- 新鲜度：两类来源最新内容距今
SELECT source_kind, max(published_at) newest_pub, max(created_at) newest_import,
       count(*) FILTER (WHERE published_at >= now()-interval '2 days') last2d,
       count(*) total
FROM items GROUP BY source_kind;

-- 翻译完整性：rss 有正文但没译文
SELECT count(*) FILTER (WHERE body_cn IS NULL) untranslated,
       count(*) FILTER (WHERE body_cn IS NOT NULL) translated
FROM items WHERE source_kind='rss' AND body IS NOT NULL AND body<>'';

-- 翻译质量：疑似拒翻/回英文的漏网（应为 0）
SELECT count(*) FILTER (WHERE body_cn ~ '(请提供|我会按|抱歉|无法翻译|作为一个)') refusal,
       count(*) FILTER (WHERE body_cn !~ '[一-龥]' AND length(regexp_replace(body_cn,'<[^>]+>','','g'))>20) still_english
FROM items WHERE source_kind='rss' AND body_cn IS NOT NULL;

-- 正文完整性：rss 短 body（去标签<500字）占比
SELECT count(*) FILTER (WHERE length(regexp_replace(body,'<[^>]+>','','g'))<500) short,
       count(*) total
FROM items WHERE source_kind='rss' AND body IS NOT NULL;

-- 富集完整性：present items 缺字段
SELECT count(*) FILTER (WHERE title IS NULL OR title='') no_title,
       count(*) FILTER (WHERE summary IS NULL) no_summary,
       count(*) FILTER (WHERE category IS NULL) no_category,
       count(*) FILTER (WHERE score IS NULL) no_score,
       count(*) FILTER (WHERE score < 1 OR score > 5) bad_score
FROM items WHERE present=true;

-- 评分分布（1-5 应有梯度）
SELECT score, count(*) FROM items WHERE present=true GROUP BY score ORDER BY score;

-- 模型标记分布
SELECT COALESCE(body_cn_model,'(null)'), count(*) FROM items
WHERE source_kind='rss' AND body_cn IS NOT NULL GROUP BY 1 ORDER BY 2 DESC;
```

管道在跑的检查（shell）：
```bash
ssh melos 'crontab -l | sed -n "/aihot-begin/,/aihot-end/p"'          # 有 importmp/pulse/gendaily/hotpass
ssh melos 'tail -3 /root/aihot/log/pulse.log; tail -3 /root/aihot/log/importmp.log'
ssh melos 'find /root/wechat-corpus -name "*.md" -printf "%TY-%Tm-%Td\n" | sort -r | head -1'  # 语料新鲜度
ssh melos 'grep MP_CORPUS /root/aihot/bin/aihot-cron.sh'              # 指向 /root/wechat-corpus
ssh melos 'supervisorctl status aihot-server'                         # RUNNING
```

API 冒烟（示例）：
```bash
B=http://127.0.0.1:8991
ssh melos "curl -s -o /dev/null -w '%{http_code}\n' $B/healthz"                      # 200
ssh melos "curl -s -o /dev/null -w '%{http_code}\n' '$B/api/public/items?source_kind=email'"  # 400
ssh melos "curl -s '$B/api/public/items?mode=all&source_kind=mp&take=3' | head -c 400"
```

---

## 6. 也要跑仓库测试（回归基线）

```bash
cd /Users/didi/aihot-internal/server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test   # 全绿
cd /Users/didi/aihot-internal/web && npx vitest run && npm run build                              # 全绿 + 构建通过
```
若有失败，记进报告——这也是"功能完整性"的一部分。

---

## 7. 交付物（报告格式）

一份 markdown 报告，含：
1. **体检表**：§5 每条探针的实测数字 + 判定（✅正常 / ⚠️可疑 / ❌异常）。
2. **功能矩阵**：§3.A 每个端点/控件/参数的 pass/fail。
3. **已知失败模式复核**：§4 六条逐条"不复现/复现"。
4. **鲁棒性**：§3.C 边界与并发结果（有无 5xx/panic/串页）。
5. **发现的新问题**：按严重度排序，每条给复现步骤 + 实际/期望。
6. **建议**：尤其针对"静默失败"——建议加哪些自动告警/监控（例如 mp 新鲜度看门狗）。

---

## 8. 硬约束（**务必遵守**）

- **生产库 `aihot` 绝不 TRUNCATE/DELETE/DROP**；破坏性测试只在 `aihot_test`。健康探针全是只读 SELECT。
- **LLM key 保密**：不打印、不写文件、不提交。（key 在 Melos `/root/wechat-push/.env`，服务通过 `aihot-server.sh` 包装注入；取值必须 `tr -d "'"` 而非 `tr -d "\x27"`——后者会删掉 key 里的 x/2/7 字符，是之前踩过的坑。）
- **不要压重译端点/pulse**（吃共享 LLM key 的 RPM，会挤占线上）。只读 API 可压。
- **不替用户发任何消息**（D-Chat/微信/邮件都不发）。
- 路径用完整绝对路径，不用 `~`。
- 改了任何东西要如实报告；跳过的项要说明。

---

## 9. 关键文件索引

- 路由装配：`server/common/server/httpserv/httpserv.go`
- 信息流参数/查询：`server/internal/publicapi/params.go`、`server/internal/items/query.go`
- 详情页 SSR：`server/internal/detailpage/{page.go,handler.go,markdown.go}`
- 翻译校验：`server/internal/pipeline/translate.go`（`looksUntranslated`）
- 全文抓取：`server/internal/ingest/{article.go,ogfetch.go}`
- 富集/评分：`server/internal/pipeline/{enrichment.go,processor.go}`
- 前端信息流：`web/src/components/Feed.tsx`、`web/src/api/items.ts`
- 设计文档：`docs/superpowers/specs/2026-07-*`（本轮各功能的 spec）
- 管道排查参考：记忆 [[aihot-mp-ingestion-chain]]、[[aihot-internal-project]]
