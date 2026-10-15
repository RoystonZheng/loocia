# aihot Melos 部署 runbook

## 架构
全管线跑在 Melos(`ssh melos`,Debian13/x86_64):本机 PG 生产库 `aihot`
(测试库 `aihot_test` 不受影响),LLM key 复用 `/root/wechat-push/.env`。
Mac 只负责交叉编译 + scp。

## 定时(crontab,标记块 `# aihot-begin/end`)

截至 **2026-09-23**，线上 Melos 的实际 crontab 仍是上一版：

```cron
25 * * * * /root/aihot/bin/aihot-cron.sh importmp -corpus /root/wechat-corpus
*/30 * * * * /root/aihot/bin/aihot-cron.sh pulse
*/30 * * * * /root/aihot/bin/aihot-cron.sh discovertools weekly -actor cron
```

这意味着当前线上 `discovertools` 还按旧的 `weekly` 入口运行，尚未使用配置中的
daily/weekly 时间槽。执行本仓库的 `install-melos.sh` 后，只有这一行会切换为：

```cron
* * * * * /root/aihot/bin/aihot-cron.sh discovertools scheduled -actor cron
```

这一分钟级任务只是调度时钟：它每分钟检查一次，到达配置的北京时间才执行；
资讯采集仍由 `pulse` 每 30 分钟运行。更新后的 wrapper 会在每次 `pulse` 成功后
顺序执行 `gendaily` 和 `hotpass`，让迟到的当天资讯及时进入日报和热点；词云直接读取
`pulse` 富化时写入的词项，因此会随这次采集立即更新。

| 单元 | 频率 | 作用 |
|---|---|---|
| pulse | 每 30 分钟 | RSS 采集 + LLM 加工(≤100 条/次) |
| discovertools scheduled | 每分钟唤醒一次 | 按配置的北京时间执行 daily/weekly 发现 |
| discovertools snapshot | 每周一 02:45 | 刷新工具 GitHub 数据并保存 Stars 快照 |
| gendaily | 每次 pulse 成功后 | 重生成昨天和当天(北京时间)日报,upsert 幂等 |
| hotpass | 每次 gendaily 后 | 聚类 + 热度重算(72h 窗口) |

## 发布 / 更新
    ./deploy/build-linux.sh     # Mac 交叉编译 server + pipeline commands → deploy/out/
    ./deploy/install-serve-melos.sh  # 先更新 server，启动时幂等补齐新 schema
    ./deploy/install-melos.sh   # 再更新任务二进制、wrapper 和 crontab(幂等)

默认部署目标是 `root@10.190.12.242`，也可以通过第一个参数或
`AIHOT_DEPLOY_HOST` 覆盖。

## 手动跑一次 / 看日志(在 Melos 上)
    /root/aihot/bin/aihot-cron.sh pulse  # 成功后自动刷新日报和热点
    /root/aihot/bin/aihot-cron.sh gendaily  # 需要单独补算时使用
    /root/aihot/bin/aihot-cron.sh hotpass  # 需要单独补算时使用
    /root/aihot/bin/aihot-cron.sh discovertools scheduled -actor cron
    tail -50 /root/aihot/log/pulse.log

## Mac 上看真实数据
    ssh -N -L 5432:localhost:5432 melos &        # 隧道
    cd server && AIHOT_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot' make run &
    cd web && npm run dev                        # localhost:5173

## 回滚 / 停用
    ssh melos 'crontab -l | sed "/# aihot-begin/,/# aihot-end/d" | crontab -'

## 已知边界
- 源清单内嵌默认覆盖 OpenAI、TechCrunch、Simon Willison、MIT Technology Review、
  Anthropic、DeepMind、Cloudflare、NVIDIA、人人都是产品经理、极客公园、
  Reddit LocalLLaMA 和 AIHOT；Reddit MachineLearning 默认 disabled。加源:
  上传 JSON 到 /root/aihot/etc/sources.json，并在运行环境设置
  `AIHOT_SOURCES_FILE=/root/aihot/etc/sources.json`；`aihot-cron.sh pulse`
  在未显式传参时会自动补 `-sources "$AIHOT_SOURCES_FILE"`。手工传入
  `pulse -sources ...` 仍优先。arXiv 量大费 LLM,慎加。
- 失败条目留在 raw_items 未处理态,下轮 pulse 自动重试;毒条目不会死循环
  (无进展即停批)。
- LLM key 轮换后无需改动(每次执行时从 .env 现读)。
- 如果生产机无法直连 Reddit 等外部来源，可在 `/root/wechat-push/.env` 增加
  `AIHOT_SOURCE_HTTP_PROXY=http://127.0.0.1:端口`。该代理只用于信息源请求，
  不会改写内部 LLM 请求；配置后下一轮 `pulse` 会自动使用。
- AI Tool 使用 `AI_TOOL_GITHUB_TOKEN` 访问 GitHub API。上线前把 token 写入
  `/root/wechat-push/.env`，不要写进仓库；可选配置 `AI_TOOL_GITHUB_MAX_PAGES` 控制单次查询分页上限，
  `AI_TOOL_GITHUB_REQUEST_INTERVAL_MS` 控制 GitHub 请求间隔，`AI_TOOL_STAR_SNAPSHOT_LIMIT`
  控制每次补采已有工具 Star 快照的上限。
- `discovertools scheduled` 每分钟检查启用的 daily/weekly 配置，只在配置的北京时间到点后执行一次；daily 每天执行，weekly 按配置的周一至周日执行。旧的 `discovertools weekly` 命令仍保留用于兼容和手动排障。
- 线上旧表在 **2026-09-23** 尚未包含 `schedule_time` 和 `schedule_weekday`。新 `server` 或
  `discovertools` 启动时会通过 `EnsureSchema` 幂等补齐字段和约束；部署后首次启动会完成这一步。

## 24/7 托管(nginx + systemd,无需 Mac)

全站常驻在 Melos:Go 服务器(API + SSR 详情页)由 supervisor 拉起,前端静态由 nginx
托管,nginx 反代动态前缀到 Go。访问入口:**http://10.190.12.242:8899/**

架构(注:Melos 的 PID 1 是 **supervisord**,不是 systemd):
- supervisor 程序 `aihot-server`(`/etc/supervisor/conf.d/aihot-server.conf`,
  autorestart)→ `/root/aihot/bin/server` 连本机 `localhost:5432/aihot`,监听 `:8991`。
- nginx server block(`:8899`,`/etc/nginx/conf.d/aihot.conf`):`/` 托管
  `/var/www/aihot`(SPA,放 /var/www 因 nginx www-data 读不了 /root),
  `/api/` `/items/` `/healthz` 反代 `127.0.0.1:8991`。不动现有 `:80` default_server。

发布 / 更新:
    ./deploy/build-linux.sh          # 交叉编译 server+3命令 + 构建 web/dist
    ./deploy/install-serve-melos.sh  # scp + supervisor restart + nginx -s reload

仅更新前端:重跑 build-linux.sh 后 `scp -r deploy/out/web/. melos:/var/www/aihot/`。
仅更新后端:`scp deploy/out/server melos:/root/aihot/bin/server && ssh melos 'supervisorctl restart aihot-server'`。

运维:
    ssh melos 'supervisorctl status aihot-server'  # 服务状态
    ssh melos 'tail -50 /root/aihot/log/server.log'# 应用日志
    ssh melos 'supervisorctl restart aihot-server' # 重启
    ssh melos 'supervisorctl tail -f aihot-server' # 跟日志

停用:
    ssh melos 'supervisorctl stop aihot-server; rm /etc/supervisor/conf.d/aihot-server.conf; supervisorctl reread; supervisorctl update'
    ssh melos 'rm /etc/nginx/conf.d/aihot.conf; nginx -s reload'
