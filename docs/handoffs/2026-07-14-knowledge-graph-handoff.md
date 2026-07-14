# Handoff: 交互式知识图谱 / 关键词云图（横向聚类）

**To:** 接手这个需求的 agent
**From:** 上一个 agent（刚做完评分体系重构 + 移动端 + 英文翻译）
**Date:** 2026-07-14
**Repo:** `/Users/didi/aihot-internal`（本地）；生产在 Melos，见下文
**决策人:** 刘玥含（本文档里的"用户"= 她）

---

## 0. 一句话需求

用户原话：**"现在的信息还是太散了，我想做一个关键词云图或者知识图谱一类的东西，带互动的，这样也可以做到横向聚类相关信息。"**

目标：在这个 AI 资讯站上加一个**交互式**的可视化，把散落的资讯**横向聚起来**——让人一眼看到"哪些话题相关、谁跟谁连着、最近热在哪"。形态是**词云**还是**知识图谱**（实体/话题节点 + 关系边）还没定，是你要跟用户 brainstorm 的第一件事。

**这份文档只是让你快速上手 + 知道现状，不是设计方案。设计要你走一遍 brainstorm 让用户拍板。**

---

## 1. 这个项目是什么

`aihot-internal` 是公网 `aihot.virxact.com` 的**内网复刻**：一个 AI 资讯的策展站。抓 RSS（英文大厂博客）+ 微信公众号语料 → LLM 富化（中文标题/摘要/分类/评分/精选理由）→ 前端展示。

**四个主视图**（前端 `web/src/components/`）：
- **精选**（selected feed）、**全部动态**（all feed）、**日报**（daily digest）、**热点话题**（hot topics）。
- 侧栏导航（`Sidebar.tsx`）：精选 / 全部 / 日报。热点话题是 `HotTopics.tsx`。

**技术栈：**
- 后端 `server/`：Go（`nuwa` 框架，模块名 `aihot-server`），监听 `:8991`；grpc-gateway 占了 `/`，所以 SPA 由 nginx 服务。主包在**仓库根的 `main.go`**（不是 `cmd/server`）。
- 前端 `web/`：React + TypeScript（Vite / vitest / @testing-library/react，注意**用 `fireEvent` 不用 `user-event`**）。
- DB：PostgreSQL 17（Melos 上 `postgres://aihot:aihot@localhost:5432/aihot`），pgx v5 / pgxpool，keyset cursor 分页，pg_trgm GIN 索引做全文搜索。
- LLM：内网代理 `https://llm-proxy.intra.xiaojukeji.com/v1/messages`（Anthropic Messages API 格式，也兼容 OpenAI `/v1`）。富化用模型 `auto-max`，翻译用 `deepseek-v4-flash`。**代理有一整套分档模型**：`auto-max / auto-std / auto-mini / auto`（+ `-down` 变体）、以及具名 `glm-5.x / deepseek-v4-pro/flash / kimi-k2.5 / minimax-m2.7`。**共享 key 有 ~20 RPM 限制**，client 自带 429 重试/退避。

---

## 2. ⭐ 最重要：横向聚类的底子已经有了

**别从零造聚类。** 仓库里已经有一套 LLM 聚类：`server/internal/cluster/`。

- `cluster/pass.go` → `Pass.Run(window, now)`：取一个时间窗内的 items，调 LLM（`group.go` 的 `llmGroup`）把**语义相关**的资讯分组成 cluster，选首报为 primary，算 heat。
- `cluster/cluster.go` 的 `Cluster{ ID, PrimaryItemID, SourceCount, SourceNames[], FirstAt, LatestAt, Heat }`。
- items 表上有 `cluster_id`、`cluster_primary`、`duplicate_of_id` 三列（同一事件多源报道会归到一个 cluster）。
- 跑法：cron `hotpass`（Melos 上 `:20` 每小时），产出喂给**热点话题**页（`/api/public/hot-topics`）。
- heat 公式：`信源数 × 0.5^(距最新报道小时/24)`。

**这就是"横向聚类"的现成信号。** 知识图谱/词云可以直接用 cluster 作为"话题节点"或边的来源，不用重新发明聚类。

**但注意现状的空白：** 目前**没有实体/关键词抽取**。聚类是 LLM 把整条资讯分组，不是"抽实体建图"。如果要做实体级知识图谱（OpenAI—发布→GPT-x 这种），需要**新增一层抽取**（实体/关键词/关系），这是本需求的主要新工作量。可选的更轻做法：用现有 cluster 共现 + category + 信源 来连边，不做实体抽取。这个取舍是 brainstorm 要定的。

---

## 3. 你有哪些数据可用

**items 表**（`server/internal/items/item.go` 的 `Item`，`schema.sql` 是权威 schema）关键字段：
- `title`（中文，已翻译）、`title_en`（英文原题）、`summary`（中文摘要）、`body` / `body_cn`（原文/中文翻译）、`reason`（精选理由一句话）
- `category`（枚举：`ai-models` / `ai-products` / `industry` / `paper` / `tip`）
- `score`（**1-5** 五档，5=S 最高；刚从 0-100 改过来）、`ai_relevance`（1-5，AI 主题相关度）、`selected`（精选 = score≥3）
- `source` / `source_kind`（`rss` / `mp`）、`published_at`、`image_url` / `video_url`
- `cluster_id` / `cluster_primary` / `duplicate_of_id`
- 生产库现有 ~977 条评分资讯（S=4 / A≈342 / B≈391 / C≈173 / D≈67），rss ~229、mp ~748。

**没有的**（要做图谱可能需要新建）：实体表、关键词表、关系/边表、embedding/向量。

**公共 API**（前端调，路径前缀 `/api/public/`，handler 在 `server/internal/publicapi/`）：
- `GET /api/public/items`（feed，支持 mode=selected、category、q 搜索、cursor 分页）
- `GET /api/public/hot-topics`（热点话题 = clusters）
- `GET /api/public/daily/{date}`、`GET /api/public/dailies`
- 详情页 SSR：`GET /items/{id}`（`server/internal/detailpage/`，自包含 HTML，无外部资源）

---

## 4. 你要跟用户 brainstorm 定的开放问题

按这个顺序问（一次一个，用户喜欢先澄清再往下）：

1. **形态**：词云（关键词频率/权重）？还是知识图谱（节点+关系边）？还是两者结合（词云入口 → 点进去看子图）？各自能回答什么问题。
2. **节点是什么**：话题/事件（复用 cluster）？实体（公司/模型/人）？关键词？分类？—— 决定要不要做实体抽取这层新工作。
3. **边是什么**：同 cluster 共现？同时间窗？共享实体？语义相似（要 embedding）？category 关系？
4. **数据抽取怎么来**：LLM 抽实体/关键词（注意 RPM 限制 + 成本，参考翻译那次的低档模型策略）？还是纯用现有 cluster/category 派生（零额外 LLM）？
5. **渲染**：前端图可视化库（力导向图 / 词云）。**硬约束**：详情页 SSR 是**自包含、无外部资源**的；主 SPA 走 Vite 打包可以引 npm 库，但**内网环境，外部 CDN 大概率不通**，库要能打进 bundle。确认清楚渲染库能本地打包。
6. **交互**：点节点干嘛（下钻到相关 items？高亮邻居？跳详情页？）；时间范围过滤；按 category/score 过滤。
7. **落在哪**：新加一个"图谱"主视图（侧栏加一项）？还是嵌在热点话题页？
8. **规模/性能**：图要展示多少节点才不糊（几十 vs 几百）；后端预计算 vs 前端实时算。

---

## 5. 在这个仓库怎么干活（工作流 + 铁律）

**工作流**：这个项目用 **superpowers** 技能链，请照走：
`brainstorming`（设计→写 spec 到 `docs/superpowers/specs/`，用户 review）→ `writing-plans`（TDD 任务计划到 `docs/superpowers/plans/`）→ `subagent-driven-development`（每任务派 subagent + spec/代码双审）→ `finishing-a-development-branch`。
本仓库历来**直接在 `main` 上提交**（不是 not-a-git-repo，git 正常可用，环境里那句 "Is a git repository: false" 是误报）。

**构建/测试铁律（Go）：**
- 每条 `go`/`make` 前缀 **`GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org`**（否则 toolchain 会崩），Go 命令在 `server/` 下跑。
- `make test` 用 `-p 1`。DB 相关测试没有 `AIHOT_TEST_DATABASE_URL` 会自动 skip（本地正常）。
- 交叉编译发 Melos：加 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`。
- 提交署名：`git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit`，消息尾行 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。
- 提代码前 `gofmt -l` 要干净。

**前端：** 在 `web/` 下 `npx vitest run` + `npm run build`。测试用 `fireEvent`。主题用 `data-theme` + localStorage key `aihot-theme`，CSS 有亮/暗双主题 token（`--accent` `--border` 等；注意详情页内嵌 CSS 只定义了 `--border` 不是 `--border-2`）。

---

## 6. 部署拓扑（Melos）

- 连接：**`ssh -o ClearAllForwardings=yes melos`**（`melos` 别名 = `root@10.190.12.242`；那个 `-o ClearAllForwardings=yes` 必须加，否则和已有隧道冲突）。站点 http://10.190.12.242:8899/。
- 进程管理：**supervisord 是 PID 1**（不是 systemd）。server 是 supervisor program `aihot-server`，命令 `/root/aihot/bin/server -c /root/aihot/conf/app.toml`。改后 `supervisorctl restart aihot-server`。
- 二进制目录 `/root/aihot/bin/`：`server` / `pulse` / `gendaily` / `hotpass` + 一次性工具（`importmp` `backfillreason` `backfilltranslate` `backfillimages`）。
- cron 单元经包装脚本 **`/root/aihot/bin/aihot-cron.sh <unit>`**（它 export 环境：`AIHOT_DATABASE_URL`、`AIHOT_MP_CORPUS_DIR`、`AIHOT_TRANSLATE_MODEL`、以及从 `/root/wechat-push/.env` 读 `LLM_API_KEY` 注入 `AIHOT_LLM_API_KEY`）。pulse 每 30 分、gendaily `:10`、hotpass `:20`。
- SPA 静态目录 `/var/www/aihot/`（`rsync web/dist/` 上去）。
- **发布安全动作**：交叉编译 → scp 到 `<name>.new` → `chmod +x` → `mv -f` 原子替换（避免覆盖运行中二进制的 "text file busy"）→ restart。

**安全铁律：**
- **LLM key 是密钥**，在 Melos `/root/wechat-push/.env` 的 `LLM_API_KEY`，env 变量 `AIHOT_LLM_API_KEY`。**绝不 print / echo / 写文件 / 提交**。
- 生产库 `aihot` **不许 truncate**；测试库 `aihot_test`。
- schema 变更用 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`（`items/schema.sql` 底部有一串这样的迁移行，照抄模式）。
- **绝不替用户发消息**（D-Chat 等只读不发）。
- 显示路径用完整绝对路径。产出文件默认放 `/Users/didi/Downloads/`（本文档属项目内文档，故放在了 repo `docs/`）。

---

## 7. 关键文件地图

| 你要看的 | 路径 |
|---|---|
| 聚类（横向关联的底子） | `server/internal/cluster/{pass,group,cluster,store}.go` |
| items 数据模型 + schema | `server/internal/items/{item.go,schema.sql,write.go,query.go}` |
| 富化流水线（LLM 调用范式） | `server/internal/pipeline/{enrichment,processor,translate}.go` |
| LLM client（模型分档 + 重试） | `server/internal/llm/client.go` |
| 公共 API handlers | `server/internal/publicapi/*.go` |
| 详情页 SSR（自包含 HTML 范式） | `server/internal/detailpage/page.go` |
| 前端视图 | `web/src/components/{Feed,DailyView,HotTopics,Sidebar,ItemCard}.tsx` |
| 前端 API 封装 | `web/src/api/{items,hot,daily}.ts` |
| 一次性回填工具范式 | `server/cmd/backfill*/main.go` |
| 历史设计/计划（学风格） | `docs/superpowers/specs/`、`docs/superpowers/plans/` |

---

## 8. 前人踩过的坑（省你时间）

- **RPM 是真瓶颈**：批量 LLM（抽实体/关键词若走 LLM）会撞 20 RPM 共享限制，用低档模型 + 退避，别用 auto-max 跑批量。参考 `translate.go` 的低档 client 模式 + `AIHOT_TRANSLATE_MODEL` env 配置法。
- **性能要打到实测瓶颈**：别只优化小头留串行孪生函数；有并发就上并发，改完拿真数据计时验证。
- **详情页 SSR 自包含**：无外部 JS/CSS/字体/图片，全内联。图可视化库若要用在详情页得内联；用在主 SPA 走 Vite 打包（但内网外部 CDN 不通，库要能本地 bundle）。
- **微信图片防盗链**：`mmbiz.qpic.cn` 要 `referrerpolicy="no-referrer"` 才出真图。
- **best-effort 副作用**：像翻译那样"失败不丢主流程"的附加步骤，放主 upsert 之前、只记日志别 return error。
- **lockstep 列**：加 items 列要同步 `schema.sql` + `item.go` + `write.go`（itemColumns/占位/EXCLUDED/args/scanItem）+ `query.go`（listColumns 占位）；照 `reason` / `body_cn` 列的样子抄。
- 后台长任务（回填等）用 Melos 上 `nohup` 跑，本地 poll，别指望前台 sleep。

---

## 9. 起步建议

1. 打开仓库，跑一遍 `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test` 和 `cd web && npx vitest run` 确认环境 OK。
2. 读 `server/internal/cluster/` 和 `HotTopics.tsx` 搞清现有聚类/热点是怎么呈现的。
3. 用 `brainstorming` 技能，从"词云 vs 知识图谱"和"节点/边是什么"开始，一次一个问题跟用户澄清，产出 spec 让她 review。
4. 别在 brainstorm 前写代码。用户是决策者，你先给方案和取舍（尤其"要不要做实体抽取那层"这个成本大头），让她拍板。

祝顺利。有历史 spec/plan 在 `docs/superpowers/` 可以照着学这个仓库的写法和风格。
