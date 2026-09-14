# aihot Melos 部署 runbook

## 架构
全管线跑在 Melos(`ssh melos`,Debian13/x86_64):本机 PG 生产库 `aihot`
(测试库 `aihot_test` 不受影响),LLM key 复用 `/root/wechat-push/.env`。
Mac 只负责交叉编译 + scp。

## 定时(crontab,标记块 `# aihot-begin/end`)
| 单元 | 频率 | 作用 |
|---|---|---|
| pulse | 每 30 分钟 | RSS 采集 + LLM 加工(≤100 条/次) |
| discovertools | 跟随 pulse，每 30 分钟 | AI Tool weekly 配置发现 + Stars 快照 |
| gendaily | 每小时 :10 | 重生成当天(UTC)日报,upsert 幂等 |
| hotpass | 每小时 :20 | 聚类 + 热度重算(72h 窗口) |

## 发布 / 更新
    ./deploy/build-linux.sh     # Mac 交叉编译 server + pulse/gendaily/hotpass/discovertools → deploy/out/
    ./deploy/install-melos.sh   # scp + 刷新 crontab(幂等)

## 手动跑一次 / 看日志(在 Melos 上)
    /root/aihot/bin/aihot-cron.sh pulse
    /root/aihot/bin/aihot-cron.sh discovertools weekly -actor cron
    tail -50 /root/aihot/log/pulse.log

## Mac 上看真实数据
    ssh -N -L 5432:localhost:5432 melos &        # 隧道
    cd server && AIHOT_DATABASE_URL='postgres://aihot:aihot@localhost:5432/aihot' make run &
    cd web && npm run dev                        # localhost:5173

## 回滚 / 停用
    ssh melos 'crontab -l | sed "/# aihot-begin/,/# aihot-end/d" | crontab -'

## 已知边界
- 源清单默认 OpenAI Blog + Google AI Blog(内嵌);加源:上传 JSON 到
  /root/aihot/etc/sources.json 并把 pulse 的 cron 行改成
  `... pulse -sources /root/aihot/etc/sources.json`。arXiv 量大费 LLM,慎加。
- 失败条目留在 raw_items 未处理态,下轮 pulse 自动重试;毒条目不会死循环
  (无进展即停批)。
- LLM key 轮换后无需改动(每次执行时从 .env 现读)。
- AI Tool 使用 `AI_TOOL_GITHUB_TOKEN` 访问 GitHub API。上线前把 token 写入
  `/root/wechat-push/.env`，不要写进仓库；可选配置 `AI_TOOL_GITHUB_MAX_PAGES` 控制单次查询分页上限，
  `AI_TOOL_GITHUB_REQUEST_INTERVAL_MS` 控制 GitHub 请求间隔，`AI_TOOL_STAR_SNAPSHOT_LIMIT`
  控制每次补采已有工具 Star 快照的上限。
- `discovertools weekly` 目前会执行所有启用的 weekly 配置，不做“每周只跑一次”的冷却判断；跟随 `pulse` 后就是每 30 分钟触发一次。

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
