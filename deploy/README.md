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
