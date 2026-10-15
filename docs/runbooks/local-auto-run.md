# 本地自动运行验证

## 适用场景

用于验证工具发现代码更新后，是否能在和生产相同的调度入口下运行。这里的本地数据库固定为
`aihot_local_prod_clone`，不会连接生产库。

## 生产触发器对照

2026 年 9 月 23 日线上实际使用的是：

```cron
*/30 * * * * /root/aihot/bin/aihot-cron.sh discovertools weekly -actor cron
```

这是旧入口。更新本仓库后，`deploy/install-melos.sh` 会安装：

```cron
* * * * * /root/aihot/bin/aihot-cron.sh discovertools scheduled -actor cron
45 2 * * 1 /root/aihot/bin/aihot-cron.sh discovertools snapshot
```

`scheduled` 每分钟只检查时钟；只有启用的 daily/weekly 配置到达自己的北京时间
`schedule_time`，才会真正启动一次发现。weekly 还会额外匹配 `schedule_weekday`。
Stars 快照由独立的 `snapshot` 任务执行，不跟每分钟调度绑在一起。
如果同一配置上一轮还在运行，下一次唤醒会记录 `already running` 并跳过，避免并发写入。

## 安装本地副本

在仓库根目录执行：

```bash
./server/scripts/install-local-aihot.sh
```

它会完成两件事：

1. 安装 `com.aihot.local-server` LaunchAgent，后端异常退出后自动拉起，监听
   `127.0.0.1:8991`。
2. 安装当前用户的 crontab：
   - `pulse` 每 30 分钟执行一次，抓取资讯并处理本地库里的待富化内容；成功后顺序追跑
     `gendaily` 和 `hotpass`。
   - `discovertools scheduled` 每分钟检查工具配置是否到点。
   - `discovertools snapshot` 每周一 02:45 刷新 Stars 快照。
   - 词云直接读取 `pulse` 富化时写入的词项，采集完成后无需额外的词云任务。

由于当前仓库位于 `~/Documents/Codex`，macOS 后台进程可能没有读取该目录的权限。
安装脚本会先从当前工作树构建 `server`、`pulse`、`gendaily`、`hotpass` 和
`discovertools`，再暂存到
`~/Library/aihot-local`，由 LaunchAgent 和 cron 执行暂存版本。更新代码后重新执行
`restart-local-aihot.sh`，就会重新构建并接管新版本。

本地触发器只读取当前进程环境和
`~/Library/aihot-local/local.env`，不会 SSH 到生产机取密钥。数据库 DSN 会强制校验为
`aihot_local_prod_clone`。因此本地 `pulse` 会访问真实资讯源并调用真实 LLM，结果只写入
本地库。没有 LLM key 时，pulse 会自动降级为 `ingest-only`，仍抓取并写入 `raw_items`，
但不会把未评分的原始条目伪装成 `items`；配置凭据后下一轮继续富化。`gendaily` 可以
在没有 LLM key 时生成无导语日报，`hotpass` 仍需要 LLM key。

常驻 `8991` 服务和 cron 使用同一个 `local.env`。所有 LLM 调用固定发送模型
`deepseek-v4-flash`；`AIHOT_LLM_MODEL`、`AIHOT_TRANSLATE_MODEL`、
`AIHOT_TRANSLATE_RETRY_MODEL`、`AIHOT_TERMS_MODEL` 仅为历史兼容变量，不会把运行模型切换到
`auto-*`。历史工具用途重分类默认关闭，避免服务启动时阻塞或消耗批量 LLM 配额；需要时显式设置
`AI_TOOL_PURPOSE_RECLASSIFY_LIMIT` 或运行 `discovertools reclassify-purpose`。

如需让本地自动发现真正访问 GitHub 和 LLM，可在本机创建权限为 `600` 的
`~/Library/aihot-local/local.env`，只放本机测试凭据，例如：

```bash
AIHOT_LLM_API_KEY=...
AI_TOOL_GITHUB_TOKEN=...
```

没有 GitHub token 时，`discovertools scheduled` 仍会按配置检查到期任务，并可使用
GitHub 匿名额度执行；没有 LLM key 时，工具用途分类会退化为关闭，资讯 `pulse` 会进入
本地原始采集模式。凭据缺失和网络失败都不会改变本地数据库隔离规则。

## 更新代码后

```bash
./server/scripts/restart-local-aihot.sh
```

工具调度会执行最近一次暂存的 `discovertools` 二进制。后端通过 LaunchAgent 重启后
读取最近一次暂存的代码和 schema。

`pulse` 使用最近一次暂存的 `pulse` 二进制，频率与生产一致，为每 30 分钟一次。若要
测试公众号语料，可以设置 `AIHOT_LOCAL_MP_CORPUS_DIR`；默认只跑内置 RSS/HTML/AIHOT
来源。`gendaily` 和 `hotpass` 也会随安装脚本暂存到 `~/Library/aihot-local/bin`，
由每次成功的 `pulse` 自动调用；需要补算时仍可通过 `local-aihot-unit-trigger.sh` 单独运行。

## 验证

```bash
curl -fsS http://127.0.0.1:8991/healthz
crontab -l | sed -n '/# aihot-local-tools-begin/,/# aihot-local-tools-end/p'
launchctl print "gui/$(id -u)/com.aihot.local-server"
tail -f "$HOME/Library/Logs/aihot/discovertools.log"
tail -f "$HOME/Library/Logs/aihot/pulse.log"
tail -f "$HOME/Library/Logs/aihot/gendaily.log"
tail -f "$HOME/Library/Logs/aihot/hotpass.log"
```

数据库中检查最近运行：

```sql
SELECT id, config_id, trigger_source, actor, status, started_at, finished_at,
       result_count, pages_scanned, error_class
FROM tool_discovery_runs
ORDER BY started_at DESC
LIMIT 10;
```

检查配置是否到了执行时间：

```sql
SELECT id, name, trigger_mode, schedule_time, schedule_weekday,
       enabled, last_run_at, last_run_status
FROM tool_discovery_configs
WHERE deleted_at IS NULL
ORDER BY updated_at DESC;
```

## 手动复现

不等 cron，可以直接执行和生产相同的命令：

```bash
./server/scripts/local-aihot-cron-safe.sh discovertools scheduled -actor local-manual
```

资讯可以手动跑一轮；成功后会自动刷新日报和热点：

```bash
./server/scripts/local-aihot-cron-safe.sh pulse
```

有 LLM key 时，这会使用真实资讯源和 LLM，并写入 `aihot_local_prod_clone`；没有 key
时会明确运行 `ingest-only`，只写入 `raw_items`，不写入未评分的 `items`。两种模式都不写生产库。

可以用仓库内的 `server/testdata/local-pulse-sources.json` 配合一个本地 RSS 服务做可重复的
采集 smoke test：

```bash
AIHOT_LOCAL_SOURCES_FILE="$PWD/server/testdata/local-pulse-sources.json" \
  "$HOME/Library/aihot-local/scripts/local-aihot-pulse-trigger.sh"
```

无 LLM key 时，日志中的 `pulse ingest-only` 和数据库里的 `raw_items` 是采集成功证据；
它不会让 `AI 动态` 直接展示未经加工的原文。要让 5178 的默认资讯、热点和词云出现新数据，
还必须配置可访问的 LLM key/网关，让下一轮 `pulse` 把待处理原文加工成 `items`。本地库中
只有超过 7 天的历史 `items` 时，默认资讯列表为空是预期行为；可用历史筛选验证页面，不应
把它当作采集失败。

日报可以单独验证（例如补算某次失败的派生任务）：

```bash
./server/scripts/local-aihot-cron-safe.sh gendaily
```

它会使用本地已精选资讯生成日报；没有可用日期时会记录 `no selected items, skipped`，
这表示调度入口成功但数据条件不足，不代表生成成功。

如果只想做轻量网络 smoke，可以临时限制 GitHub 请求：

```bash
AI_TOOL_GITHUB_MAX_PAGES=1 \
AI_TOOL_GITHUB_REQUEST_INTERVAL_MS=0 \
AI_TOOL_STAR_SNAPSHOT_LIMIT=1 \
./server/scripts/local-aihot-cron-safe.sh discovertools scheduled -actor local-smoke
```

## 失败处理

- `8991` 无法启动：先看 `~/Library/Logs/aihot/server.log`，再执行
  `./server/scripts/restart-local-aihot.sh`。
- 触发器没有日志：检查 `crontab -l` 和
  `~/Library/Logs/aihot/discovertools.log`、`~/Library/Logs/aihot/pulse.log`、
  `~/Library/Logs/aihot/gendaily.log`、`~/Library/Logs/aihot/hotpass.log`。
- 出现数据库拒绝：脚本只接受数据库名精确为 `aihot_local_prod_clone` 的
  `AIHOT_DATABASE_URL` 和 `AI_TOOL_DATABASE_URL`，不会接受相似名称或通过
  `AIHOT_LOCAL_DATABASE` 改库名。
- 没有执行记录但命令成功：先看配置的 `enabled`、`trigger_mode`、北京时间和
  `last_run_at`；未到时间时 `scheduled` 只会报告 skipped。
