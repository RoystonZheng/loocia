# 翻译质量修复 + 详情页 UX（返回键 / 重试翻译 / 声明模型）— Design

**Date:** 2026-07-14
**Status:** Approved (co-designed with user)
**Goal:** 修掉低档模型产生的坏译文（拒翻/回英文），并给详情页加返回键、失败重试翻译、译文模型声明。

## 背景（实测）

生产 228 条 rss 译文中 **17 条坏的（~7.5%）**，两种失败模式，`deepseek-v4-flash` 在部分（短/纯链接）正文上翻车：
- **拒翻**（1 条）：`body_cn` = "请提供需要翻译的 HTML 正文内容，我会按要求进行处理。"
- **回英文**（16 条）：`body_cn` = 原英文（模型原样返回）

根因：best-effort 只判"调用报错"，模型**成功返回垃圾**照存。核心修复 = **输出校验**，识别坏译文并当失败处理（不存 → 回退英文原文）。

---

## 1. 译文输出校验（核心 bug 修复）

在 `pipeline/translate.go`，`Translate` 成功拿到模型输出后做校验，坏则返回 error（不返回垃圾字符串）。这样所有调用方（processor best-effort、backfill、重试端点）自动都不会存坏译文。

新增 `looksUntranslated(src, out string) bool`，命中任一即坏：
- **拒翻/元回复**：out 含 `请提供` / `我会按` / `抱歉` / `无法翻译` / `请将` / `作为一个` / `as an AI` / `please provide` / `I'll translate` 等（大小写不敏感，取一组关键词）。
- **回英文**：out 去标签后长度 > 20 且 **CJK 占比 < 0.10**（几乎没中文 = 没翻）。
- **空**：out 去标签 trim 后为空。

`Translate` 逻辑：`out = TrimSpace(model output)`；`if looksUntranslated(body, out) { return "", errUntranslated }`；否则返回 out。新增哨兵 `var errUntranslated = errors.New("translation looks untranslated/refused")`。

## 2. 改进翻译提示词（降低失败率）

`translateSystemPrompt` 增补明确指令：
- "**必须输出中文译文**；即使正文很短也要翻译；若正文已是中文则原样返回。"
- "**不要**索要内容、不要解释、不要返回英文原文、不要输出除译文外的任何文字。"
保留原有的"保留 HTML 标签/术语英文/说人话"。

## 3. 记录每条译文用的模型（`body_cn_model`）

- `items` 加列 `body_cn_model TEXT`（照 reason/body_cn lockstep：schema.sql + item.go `BodyCNModel *string` + write.go 25 列 + query.go `NULL::text AS body_cn_model`）。
- `Translator` 增 `Model() string`；构造改 `NewTranslator(llm LLM, model string)`；`llm.Client` 加 `Model() string` getter；调用方 `NewTranslator(c, c.Model())`。
- 存译文时同存模型：processor / backfill / 重试端点在写 body_cn 成功时设 `it.BodyCNModel = &m`（processor 用 `p.translator.Model()`）。

## 4. 返回键（详情页）

`detailpage/page.go` 文章顶部（topbar 附近）加 `<a class="back" href="/">← 返回</a>`。href=`/`（feed），直开也能用。加一点 CSS。

## 5. 重试翻译（失败时）

**端点**：`POST /api/public/items/{id}/retranslate`（handler 在 detailpage 或 publicapi）。
- 载入 item；仅当 `source_kind=rss && body 非空` 才处理，否则 400。
- 用**重试翻译 client（auto-std，env `AIHOT_TRANSLATE_RETRY_MODEL` 默认 `auto-std`）**的 Translator 翻译 + 校验（同 §1）。
- 成功 → `UPDATE items SET body_cn=$1, body_cn_model=$2, updated_at=now()`；返回 `200 {"ok":true}`。
- 失败（仍坏/报错）→ `200 {"ok":false}`（前端提示重试仍失败）。
- 装配：server main 需构造 auto-std translator + items store，注入 handler。

**前端（详情页 SSR）**：viewModel 加 `CanRetranslate bool`（= rss && body 非空 && body_cn 缺失）。当 `CanRetranslate` 且非 HasTranslation：在"原文"区显示按钮 `翻译失败 · 点此重试翻译`（id=`tr-retry`）。内联 JS `__retranslate(id)`：fetch POST → 按钮变"翻译中…" → ok 则 `location.reload()`；否则按钮变"重试仍失败，可稍后再试"。
- item id 传给 JS：按钮 `onclick="__retranslate('{{.ID}}')"`（viewModel 加 `ID string`）。

## 6. 声明翻译模型（详情页）

当 `HasTranslation`，在译文块下方（或"原文"标题旁）显示小字：`本文由 AI（{{.BodyCNModel}}）翻译`。viewModel 加 `BodyCNModel string`（空则回退显示"AI"）。markdown 导出的 `## 原文（AI 翻译）` 同步带上模型名：`## 原文（AI 翻译 · <model>）`。

## 7. 修存量 17 条

一次性：`UPDATE items SET body_cn=NULL, body_cn_model=NULL WHERE source_kind='rss' AND body_cn IS NOT NULL AND (<拒翻或回英文条件>)`（用 §1 同款 SQL 版判据）。然后重跑 `backfilltranslate`（现在带校验 + 改进提示词）：能翻好的翻好，翻不好的留 NULL → 详情页回退英文 + 显示重试按钮（用户可手动上 auto-std 再救）。

## 测试
- `translate_test`：`looksUntranslated` 命中拒翻/回英文/空，放过正常中文；`Translate` 对坏输出返回 `errUntranslated`、对好输出透传；`Model()` 返回构造时的名字。
- `processor_test`：坏译文（fakeTranslator 返回触发校验的 or 直接 err）→ BodyCN 不设、条目仍存；好译文 → BodyCN + BodyCNModel 都设。
- `detailpage/page_test`：HasTranslation → 显示模型声明 + 返回键；rss 无 body_cn → CanRetranslate=true + 重试按钮 + 无声明；mp → 无按钮无声明；返回键始终在。
- 重试 handler test：rss 成功更新、非 rss 400、坏结果 ok:false。

## 部署
- `ALTER TABLE items ADD COLUMN IF NOT EXISTS body_cn_model TEXT`。
- **server 现状**：直接由 supervisor 启动（`command=/root/aihot/bin/server ...`），env 只有 `AIHOT_DATABASE_URL`，**没有 LLM key**。重试端点需要 key。做法：新增启动包装 `/root/aihot/bin/aihot-server.sh`（仿 aihot-cron.sh）——export `AIHOT_LLM_API_KEY`（从 `/root/wechat-push/.env` 读）+ `AIHOT_TRANSLATE_RETRY_MODEL="auto-std"` + `AIHOT_DATABASE_URL`，末尾 `exec /root/aihot/bin/server -c /root/aihot/conf/app.toml "$@"`。改 `aihot-server.conf` 的 `command=` 指向包装脚本，`supervisorctl reread && supervisorctl update && supervisorctl restart aihot-server`。key 全程不落配置文件、不打印。
- 编译发 server + pulse + backfilltranslate + web。
- 跑存量修复 SQL + 重跑 backfilltranslate。
- 验证：坏译文页现出重试按钮 → 点击 → 出中文 + 模型声明；返回键跳 feed；抽检 17 条修复情况。

## Out of scope
- 全部译文用强模型重翻（只修坏的；日常入库仍 deepseek-flash 省额度）。
- 详情页做完整 SPA 化 / 前端路由返回（返回键直接 href=/ 够用）。
- 语言自动检测（仍按 source_kind=rss）。

## 关键约束提醒
- server 进程若原本没有 `AIHOT_LLM_API_KEY`，重试端点会失败——部署前确认 supervisor 的 aihot-server 环境有 LLM key（key 保密，勿打印）。若不想给 server LLM 权限，退路：重试端点改为把该 item 标记待重译、由 pulse 下轮处理（异步）。**默认走同步端点方案**，部署时确认 key 可用。
