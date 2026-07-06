# aihot Melos 部署 runbook

## 架构
全管线跑在 Melos(`ssh melos`,Debian13/x86_64):本机 PG 生产库 `aihot`
(测试库 `aihot_test` 不受影响),LLM key 复用 `/root/wechat-push/.env`。
Mac 只负责交叉编译 + scp。

## 定时(crontab,标记块 `# aihot-begin/end`)
| 单元 | 频率 | 作用 |
|---|---|---|
| pulse    | 每 30 分钟 | RSS 采集 + LLM 加工(≤100 条/次) |
| gendaily | 每小时 :10 | 重生成当天(UTC)日报,upsert 幂等 |
| hotpass  | 每小时 :20 | 聚类 + 热度重算(72h 窗口) |

## 发布 / 更新
    ./deploy/build-linux.sh     # Mac 交叉编译 → deploy/out/
    ./deploy/install-melos.sh   # scp + 刷新 crontab(幂等)

## 手动跑一次 / 看日志(在 Melos 上)
    /root/aihot/bin/aihot-cron.sh pulse
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

## 24/7 托管(nginx + systemd,无需 Mac)

全站常驻在 Melos:Go 服务器(API + SSR 详情页)由 supervisor 拉起,前端静态由 nginx
托管,nginx 反代动态前缀到 Go。访问入口:**http://10.190.12.242:8899/**

架构(注:Melos 的 PID 1 是 **supervisord**,不是 systemd):
- supervisor 程序 `aihot-server`(`/etc/supervisor/conf.d/aihot-server.conf`,
  autorestart)→ `/root/aihot/bin/server` 连本机 `localhost:5432/aihot`,监听 `:8991`。
- nginx server block(`:8899`,`/etc/nginx/conf.d/aihot.conf`):`/` 托管
  `/root/aihot/web`(SPA),`/api/` `/items/` `/healthz` 反代 `127.0.0.1:8991`。
  不动现有 `:80` default_server。

发布 / 更新:
    ./deploy/build-linux.sh          # 交叉编译 server+3命令 + 构建 web/dist
    ./deploy/install-serve-melos.sh  # scp + supervisor restart + nginx -s reload

仅更新前端:重跑 build-linux.sh 后 `scp -r deploy/out/web/. melos:/root/aihot/web/`。
仅更新后端:`scp deploy/out/server melos:/root/aihot/bin/server && ssh melos 'supervisorctl restart aihot-server'`。

运维:
    ssh melos 'supervisorctl status aihot-server'  # 服务状态
    ssh melos 'tail -50 /root/aihot/log/server.log'# 应用日志
    ssh melos 'supervisorctl restart aihot-server' # 重启
    ssh melos 'supervisorctl tail -f aihot-server' # 跟日志

停用:
    ssh melos 'supervisorctl stop aihot-server; rm /etc/supervisor/conf.d/aihot-server.conf; supervisorctl reread; supervisorctl update'
    ssh melos 'rm /etc/nginx/conf.d/aihot.conf; nginx -s reload'
