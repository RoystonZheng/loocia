# AI Cool 本地真实环境验收

## 2026-10-10 当前工作树复测

在本地服务 `127.0.0.1:5178`、`127.0.0.1:8991` 已启动的前提下执行：

```bash
npm run test:e2e:local
```

结果：`ACCEPT-001` 至 `ACCEPT-008` 共 `8 passed`。本次复测同时确认：LaunchAgent 监听 8991、cron 使用 `scheduled/snapshot`、gendaily/hotpass 由 pulse 成功后顺序触发，以及 wrapper 拒绝非 `aihot_local_prod_clone` DSN。

同一工作树的 mock 浏览器回归结果为：`9 passed / 2 skipped`；跳过项是外部源探活和真实 PostgreSQL schema 迁移。

## 运行命令

```bash
cd web
npm run test:e2e:local
```

该命令会访问真实本地服务：

- 前端：`http://127.0.0.1:5178`
- 后端：`http://127.0.0.1:8991`
- 数据库：由后端启动环境决定，验收会检查本地触发器指向 `aihot_local_prod_clone`

不带 `AI_COOL_LIVE_ACCEPTANCE=1` 时，这组用例会自动跳过，避免误连真实本地服务。

## 覆盖范围

| 自动化用例 | 覆盖 Spaceway 场景 |
|---|---|
| `ACCEPT-001` | 本地前后端连通、5178/8991 是否可测 |
| `ACCEPT-002` | 日报、热点资讯评分结果、词云接口和词云关联 |
| `ACCEPT-003` | 全部资讯筛选、公众号来源、评分筛选、搜索 |
| `ACCEPT-004` | 工具列表、工具用途标签、发现配置、GitHub 设置读取 |
| `ACCEPT-005` | 工具和发现配置操作中的必填项未填校验 |
| `ACCEPT-006` | 本地自动采集/工具自动调度触发器是否安装并指向本地库 |

## 当前边界

这是一套本地真实环境自动验收，不会主动跑重型 GitHub 发现任务，也不会把 339 条 Spaceway 用例全部标记为通过。若要回写 Spaceway，需要先建立“Spaceway 用例 ID ↔ 自动化用例”的映射，再按执行结果逐条回填。

## Spaceway 回写准备

当前 Spaceway 测试计划：

- 测试计划：`AI Cool 全量验收测试用例`
- `testPlanId`：`1359221`
- 准入执行集 `stageId`：`690715`
- 准出执行集 `stageId`：`690717`
- 当前准入执行集用例数：`339`

已通过只读抓包确认的 Spaceway 接口：

| 用途 | 接口 | 请求要点 |
|---|---|---|
| 获取执行集 | `POST /dream/api/v2/test-plan/stages` | `{ "testPlanId": 1359221 }` |
| 获取执行集用例 | `POST /dream/api/req/case/select` | `{ "requirementId": 1359221, "pageNum": 1, "pageSize": 20, "stageId": 690715, "itemUid": "6777b71e-5eed-4c0a-8886-8da63d477db9" }` |
| 获取通过率 | `POST /dream/api/v2/case/pass-rate` | `{ "testPlanId": 1359221, "stageId": 690715 }` |
| 保存单条执行结果 | `POST /dream/api/v1/test-plan/stage/execution/save` | `{ "caseUid": "...", "reqCaseId": 6263579, "stageId": 690715, "executionResult": "PASS", "splitTags": "..." }` |
| 批量保存执行结果 | `POST /dream/api/v1/test-plan/stage/execution/batch/save` | `{ "executionResult": "PASS", "stageId": 690715, "caseUids": ["..."] }` |
| 保存执行备注 | `POST /dream/api/v1/test-plan/stage/execution/remark/save` | `{ "caseUid": "...", "reqCaseId": 6263579, "stageId": 690715, "stageNote": "..." }` |

执行结果枚举：

| Spaceway 枚举 | 含义 |
|---|---|
| `PASS` | 通过 |
| `FAIL` | 未通过 |
| `BLOCK` | 阻塞 |
| `IGNORE` | 不执行 |
| `UN_EXECUTED` | 未执行 |

这些接口已经能支撑自动化回写，但当前只能安全回写“明确映射”的用例。现有 `ACCEPT-001` 到 `ACCEPT-006` 是本地冒烟验收，不等同于 Spaceway 339 条细粒度用例逐条验证。

下一步建议：

1. 建一个 `ACCEPT-* ↔ Spaceway case` 映射表，只放明确等价的用例。
2. 对无法等价的场景，保留 `UN_EXECUTED`，或新增 6 条“本地自动化冒烟验收”用例专门承接自动化结果。
3. 回写脚本默认 dry-run，只打印将要写入的 Spaceway payload；确认后再开启写入。

生成 dry-run payload：

```bash
cd web
mkdir -p /tmp/ai-cool-acceptance
AI_COOL_LIVE_ACCEPTANCE=1 PLAYWRIGHT_BASE_URL=http://127.0.0.1:5178 AI_COOL_API_BASE=http://127.0.0.1:8991 npx playwright test e2e/ai-cool-local-acceptance.spec.ts --reporter=json > /tmp/ai-cool-acceptance/ai-cool-local-acceptance.json
node scripts/spaceway-local-acceptance-payload.mjs /tmp/ai-cool-acceptance/ai-cool-local-acceptance.json > /tmp/ai-cool-acceptance/spaceway-local-acceptance-payload.json
```

回写 Spaceway 时使用：

```bash
cd web
node scripts/spaceway-apply-local-acceptance.mjs --commit --payload /tmp/ai-cool-acceptance/spaceway-local-acceptance-payload.json --cookie-file <spaceway-cookie-file>
```

`--commit` 才会真实写入；不带 `--commit` 只做 dry-run 和回读验证。

本次执行结果：

- Playwright 本地验收：`6/6 passed`
- Spaceway 写入范围：只写首批明确映射的 7 条
- Spaceway 准入执行集写入前：`passed=0`，`executed=0`，`total=339`
- Spaceway 准入执行集写入后：`passed=7`，`executed=7`，`total=339`
- 其余 332 条保持原状态，未批量标记

## 2026-09-28 全量 Spaceway 执行登记

本轮按“有证据才 PASS，缺真实环境/外部条件就 BLOCK”的口径，已把准入执行集 339 条全部回写 Spaceway：

- 写入前：`passed=7`，`executed=7`，`total=339`
- 写入后：`passed=274`，`executed=339`，`total=339`
- 回读结果：`PASS=274`，`BLOCK=65`，无读回不一致
- 结果文件：`/tmp/ai-cool-acceptance/spaceway-339-results.json`
- 写入校验：`/tmp/ai-cool-acceptance/spaceway-339-write-verification.json`
- 阻塞清单：`/tmp/ai-cool-acceptance/spaceway-339-blocked.tsv`

本轮通过证据包括：

- `npm run lint`
- `npx vitest run`，`19` 个文件、`114` 个测试通过
- `npm run build`
- `go test ./...`
- `go vet ./...`
- `make build`
- `PLAYWRIGHT_BASE_URL=http://127.0.0.1:5178 npx playwright test e2e/source-expansion-cases.spec.ts e2e/ai-tool-cases.spec.ts`，`9 passed / 2 gated skipped`
- `npm run test:e2e:local`，`6 passed`
- `discovertools scheduled` 本地触发入口执行成功，日志显示 `checked=1 skipped=1 attempted=0 ... rc=0`
- 非本地副本 DSN 负向检查退出 `rc=3`，证明 wrapper 不会误写生产库

65 条阻塞主要来自四类：

- 需要独立破坏性测试库或故障注入：事务回滚、并发、审计、旧 schema 迁移、半提交恢复。
- 需要真实时间槽或生产环境：daily/weekly 到点运行、部署后触发器和数据恢复。
- 真实 LLM 额度阻塞：本地 `pulse` 能抓取并写 raw，但日志显示富化阶段 `enriched=0 failed=20`，因此评分/精选/热点端到端不能写 PASS。
- 词云细粒度实体清洗：本轮只验证词云接口和基础关联可用，没有构造独立 terms fixture 覆盖每个实体边界。

## 首批映射草案

| 自动化用例 | 可回写状态 | Spaceway 用例 | 说明 |
|---|---|---|---|
| `ACCEPT-001` | 可回写 | `SUP-CROSS-013 5178 到 8991 proxy` / `reqCaseId=6263451` / `caseUid=d3e9119b-efd7-43ca-99f0-3d8a37b3b086` | 自动化同时校验 5178 首页、8991 `/healthz` 和 5178 代理接口 |
| `ACCEPT-002` | 候选，不回写 | `SUP-SCORE-*`、`SUP-GRAPH-*` | 只覆盖热点和词云接口有数据，不覆盖每个评分/实体抽取边界 |
| `ACCEPT-003` | 候选，不回写 | `SUP-FEED-*` | 只覆盖页面筛选链路，不覆盖每个筛选参数边界 |
| `ACCEPT-004` | 可回写 | `SUP-TOOL-001 工具池默认加载` / `reqCaseId=6263565` / `caseUid=e6b2a238-6350-4930-8096-b862c6cb4ff1` | 自动化校验工具池有数据并能打开工具列表 |
| `ACCEPT-004` | 可回写 | `SUP-TOOL-002 purposeTag 筛选` / `reqCaseId=6263495` / `caseUid=facab3fc-748f-429c-a8ed-babcd1833ba2` | 自动化使用真实用途标签筛选并校验无错误 |
| `ACCEPT-005` | 可回写 | `SUP-CFG-005 actor 必填与已有配置回退` / `reqCaseId=6263997` / `caseUid=7726f2a1-8932-4aeb-99e8-132845e535c6` | 自动化新增发现配置时不填操作人，前端阻断 |
| `ACCEPT-005` | 可回写 | `SUP-TOOL-008 repoUrl 必填为空` / `reqCaseId=6263403` / `caseUid=5a0a89f1-ce15-4e33-b98b-1d68ed06d68c` | 自动化手动添加工具时不填仓库地址，前端阻断 |
| `ACCEPT-005` | 可回写 | `SUP-TOOL-011 URL 非法` / `reqCaseId=6263867` / `caseUid=96bee681-696d-4c46-ac68-b390126ffb35` | 自动化填写非法仓库地址并校验错误提示 |
| `ACCEPT-006` | 可回写 | `TC-COMBO-025 本地触发器组合锁定副本环境` / `reqCaseId=6263861` / `caseUid=f6cddbd6-341f-4180-aadd-301546d524d4` | 自动化校验 cron 与 LaunchAgent 指向本地库 |

当前最多建议自动回写上述 7 条可回写用例，其余 332 条继续保持 `UN_EXECUTED`，直到有一一对应的自动化覆盖。
