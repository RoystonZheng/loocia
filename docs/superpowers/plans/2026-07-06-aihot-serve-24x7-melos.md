# Serve aihot 24/7 from Melos (nginx + systemd) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This phase is deploy/ops — there is no new unit-testable Go code; verification is live (curl + browser against the running host).

**Goal:** Make the whole product reachable 24/7 at `http://10.190.12.242:8899/` with NO Mac and NO SSH tunnel — the Go API+SSR server runs on Melos under systemd against the local production DB, the built React SPA is served by nginx, and nginx unifies origin by reverse-proxying `/api/`, `/items/`, `/healthz` to the Go server.

**Architecture:** Probed 2026-07-06 (all green): the nuwa server cross-compiles to a static linux/amd64 binary and **boots cleanly on Melos** (healthz/items 200 against local PG — framework monitoring deps don't block startup); nginx 1.26 is already running; `:8899` is free; the intranet allows inbound on non-standard ports (:8080 reachable from Mac). The Go framework claims mux `/` for its grpc-gateway (`defaultGatewayPattern="/"`), so the SPA CANNOT be served from the Go server's root — hence nginx serves static at `/` and proxies the dynamic prefixes to Go on `127.0.0.1:8991`. A new nginx server block on `:8899` keeps the existing `:80` default_server untouched. systemd (`aihot-server.service`, Restart=always) keeps the server alive across crashes/reboots. Frontend updates are just `scp dist/` + nothing else; server updates are `scp binary` + `systemctl restart`.

**Tech Stack:** Cross-compiled Go server binary, nginx server block, systemd unit, bash deploy script; the existing `deploy/` tooling.

---

## Environment / how to run

- Cross-compile on Mac: `GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64` (already used by `deploy/build-linux.sh`; the server main at `server/.` cross-compiles statically — verified).
- Melos: Debian 13, systemd 257, nginx 1.26.3 running on `:80` (default_server, do NOT touch). PG local at `localhost:5432/aihot` (prod). LLM key at `/root/wechat-push/.env` (server does NOT need it; only pulse/gendaily/hotpass do). Install root `/root/aihot/` (`bin/`, `conf/`, `public/`, `web/`, `log/`).
- The server reads `-c <conf>` (default `./conf/app.toml`, `http_addr=":8991"`) and serves `./public` relative to its working dir → run with `WorkingDirectory=/root/aihot`.

## Verified facts (probes, 2026-07-06)

- Cross-compiled `server/.` → static ELF, boots on Melos: `healthz=200`, `api/public/items?take=1=200` with real rows. Boot log ends `[http] Server Running! addr::8991`.
- nginx 1.26.3 running; `/etc/nginx/sites-enabled/default.conf` owns `:80` (`root /var/www/html`, `server_name _`). `:8899/:8900/:9080` free. `/etc/nginx/conf.d/` exists.
- Mac→Melos `:80` and `:8080` reachable (inbound OK on arbitrary ports).

## File Structure

- `deploy/build-linux.sh` — MODIFY: also build `out/server` (from `server/.`) and the web bundle (`web/dist`).
- `deploy/aihot-server.service` — CREATE: systemd unit for the API+SSR server.
- `deploy/aihot.nginx.conf` — CREATE: nginx server block on `:8899` (static SPA + proxy to `:8991`).
- `deploy/install-serve-melos.sh` — CREATE: ship server binary + conf + public + web dist + unit + nginx conf; enable/start/reload; verify.
- `deploy/README.md` — MODIFY: add a "24/7 托管" section.

---

### Task 1: Deploy artifacts (server unit, nginx conf, build + install scripts)

**Files:**
- Modify: `deploy/build-linux.sh`
- Create: `deploy/aihot-server.service`, `deploy/aihot.nginx.conf`, `deploy/install-serve-melos.sh`
- Modify: `deploy/README.md`

- [ ] **Step 1: Extend `deploy/build-linux.sh`** to build the server binary and the web bundle in addition to the three cron binaries. Replace the file with:
```bash
#!/usr/bin/env bash
# Cross-compile the pipeline + server binaries for Melos (linux/amd64, static),
# and build the web bundle. Outputs to deploy/out/.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/deploy/out"
mkdir -p "$OUT"

echo "== go binaries (linux/amd64 static) =="
cd "$ROOT/server"
export GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64
for cmd in pulse gendaily hotpass; do
  echo "  building $cmd..."
  go build -o "$OUT/$cmd" "./cmd/$cmd"
done
echo "  building server..."
go build -o "$OUT/server" .

echo "== web bundle =="
cd "$ROOT/web"
npm run build >/dev/null
rm -rf "$OUT/web"
cp -r dist "$OUT/web"

echo "built: $(ls -1 "$OUT" | tr '\n' ' ')"
```

- [ ] **Step 2: Create `deploy/aihot-server.service`:**
```ini
[Unit]
Description=aihot API + SSR detail-page server
After=network.target

[Service]
Type=simple
WorkingDirectory=/root/aihot
Environment=AIHOT_DATABASE_URL=postgres://aihot:aihot@localhost:5432/aihot
ExecStart=/root/aihot/bin/server -c /root/aihot/conf/app.toml
Restart=always
RestartSec=3
StandardOutput=append:/root/aihot/log/server.log
StandardError=append:/root/aihot/log/server.log

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 3: Create `deploy/aihot.nginx.conf`** (installed at `/etc/nginx/conf.d/aihot.conf`; listens on `:8899`, leaves the `:80` default_server alone):
```nginx
# aihot 24/7 前端 + API/SSR 反代。前端静态托管于 /，动态前缀反代给本机 Go(:8991)。
server {
    listen 8899;
    server_name _;

    root /root/aihot/web;
    index index.html;

    # 静态 SPA:未命中的路径回退 index.html(本项目暂无客户端路由,主要是 / 与 /assets/)。
    location / {
        try_files $uri $uri/ /index.html;
    }

    # 动态:API JSON、SSR 详情页、健康检查 → Go 服务器。前缀比 location / 更长,优先匹配。
    location /api/    { proxy_pass http://127.0.0.1:8991; proxy_set_header Host $host; }
    location /items/  { proxy_pass http://127.0.0.1:8991; proxy_set_header Host $host; }
    location = /healthz { proxy_pass http://127.0.0.1:8991; }

    access_log /root/aihot/log/nginx-access.log;
    error_log  /root/aihot/log/nginx-error.log;
}
```

- [ ] **Step 4: Create `deploy/install-serve-melos.sh`:**
```bash
#!/usr/bin/env bash
# Install/refresh the 24/7 serving stack on Melos: server binary + conf + public
# + web bundle + systemd unit + nginx server block. Idempotent.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/deploy"
HOST="${1:-melos}"

[ -x out/server ] || { echo "run build-linux.sh first (out/server missing)"; exit 1; }
[ -d out/web ]    || { echo "run build-linux.sh first (out/web missing)"; exit 1; }

echo "== dirs =="
ssh "$HOST" 'mkdir -p /root/aihot/bin /root/aihot/conf /root/aihot/log /root/aihot/web'

echo "== server binary + conf + public =="
scp out/server "$HOST:/root/aihot/bin/server"
ssh "$HOST" 'chmod +x /root/aihot/bin/server'
scp -r "$ROOT/server/conf/." "$HOST:/root/aihot/conf/"
# public/ is optional (SSE demo assets); ship if present
if [ -d "$ROOT/server/public" ]; then scp -r "$ROOT/server/public" "$HOST:/root/aihot/"; fi

echo "== web bundle =="
ssh "$HOST" 'rm -rf /root/aihot/web && mkdir -p /root/aihot/web'
scp -r out/web/. "$HOST:/root/aihot/web/"

echo "== systemd unit =="
scp aihot-server.service "$HOST:/etc/systemd/system/aihot-server.service"
ssh "$HOST" 'systemctl daemon-reload && systemctl enable --now aihot-server && sleep 2 && systemctl is-active aihot-server'

echo "== nginx server block =="
scp aihot.nginx.conf "$HOST:/etc/nginx/conf.d/aihot.conf"
ssh "$HOST" 'nginx -t && systemctl reload nginx'

echo "== verify on host =="
ssh "$HOST" '
  curl -s -o /dev/null -w "server :8991 healthz=%{http_code}\n" http://127.0.0.1:8991/healthz
  curl -s -o /dev/null -w "nginx  :8899 spa=%{http_code}\n"    http://127.0.0.1:8899/
  curl -s -o /dev/null -w "nginx  :8899 api=%{http_code}\n"    "http://127.0.0.1:8899/api/public/items?take=1"
'
echo "installed. → http://10.190.12.242:8899/"
```

- [ ] **Step 5: chmod + syntax-check the scripts.**
```bash
chmod +x deploy/build-linux.sh deploy/install-serve-melos.sh
bash -n deploy/build-linux.sh deploy/install-serve-melos.sh
```
Expected: silent (no syntax errors).

- [ ] **Step 6: Append a "## 24/7 托管" section to `deploy/README.md`:**
```markdown
## 24/7 托管(nginx + systemd,无需 Mac)

全站常驻在 Melos:Go 服务器(API + SSR 详情页)由 systemd 拉起,前端静态由 nginx
托管,nginx 反代动态前缀到 Go。访问入口:**http://10.190.12.242:8899/**

架构:
- `aihot-server.service`(systemd,Restart=always)→ `/root/aihot/bin/server`
  连本机 `localhost:5432/aihot`,监听 `:8991`。
- nginx server block(`:8899`,`/etc/nginx/conf.d/aihot.conf`):`/` 托管
  `/root/aihot/web`(SPA),`/api/` `/items/` `/healthz` 反代 `127.0.0.1:8991`。
  不动现有 `:80` default_server。

发布 / 更新:
    ./deploy/build-linux.sh          # 交叉编译 server+3命令 + 构建 web/dist
    ./deploy/install-serve-melos.sh  # scp + systemd enable/restart + nginx reload

仅更新前端:重跑 build-linux.sh 后 `scp -r deploy/out/web/. melos:/root/aihot/web/`。
仅更新后端:`scp deploy/out/server melos:/root/aihot/bin/server && ssh melos systemctl restart aihot-server`。

运维:
    ssh melos 'systemctl status aihot-server'      # 服务状态
    ssh melos 'tail -50 /root/aihot/log/server.log'# 应用日志
    ssh melos 'journalctl -u aihot-server -n 50'   # systemd 日志
    ssh melos 'systemctl restart aihot-server'     # 重启

停用:
    ssh melos 'systemctl disable --now aihot-server; rm /etc/nginx/conf.d/aihot.conf; systemctl reload nginx'
```

- [ ] **Step 7: Commit.**
```bash
git add deploy/
git commit -m "feat(deploy): 24/7 serving stack (server systemd unit + nginx block + install script)"
```

---

### Task 2: Deploy + live-verify the 24/7 site (the point)

No new code — execute and prove the site is up without Mac/tunnel.

- [ ] **Step 1: Build all artifacts.** `cd /Users/didi/aihot-internal && ./deploy/build-linux.sh` → `deploy/out/` has `server pulse gendaily hotpass` (ELF) + `web/` (index.html + assets). Confirm `file deploy/out/server` → `ELF 64-bit ... x86-64`.

- [ ] **Step 2: Install the serving stack.** `./deploy/install-serve-melos.sh` → ends with `systemctl is-active` = `active`, `nginx -t` ok, and the three on-host curls: `server :8991 healthz=200`, `nginx :8899 spa=200`, `nginx :8899 api=200`. Paste the output.

- [ ] **Step 3: Verify from Mac — NO tunnel, NO local servers.** First make sure any local dev server is stopped (`lsof -ti:8991,5173 | xargs kill 2>/dev/null || true`) and the SSH tunnel is NOT required. Then:
   - `curl -si http://10.190.12.242:8899/ | head -5` → `200`, `Content-Type: text/html` (the SPA index.html).
   - `curl -s http://10.190.12.242:8899/api/public/items?take=2 | head -c 120` → real ItemList JSON.
   - `curl -si http://10.190.12.242:8899/items/<real-id> | head -6` → `200 text/html`, `X-Robots-Tag: noindex` (SSR detail page via nginx→Go). Get a real id: `ssh melos "psql 'postgres://aihot:aihot@localhost:5432/aihot' -tAc \"SELECT id FROM items WHERE present AND duplicate_of_id IS NULL ORDER BY published_at DESC LIMIT 1\""`.
   - `curl -s http://10.190.12.242:8899/api/public/hot-topics | head -c 80` and `.../api/public/daily` → 200/JSON or 404 (both valid).

- [ ] **Step 4: Browser verify the live 24/7 site (Mac, no local servers).** Playwright: `browser_navigate http://10.190.12.242:8899/` → the feed renders real Chinese items (from prod DB, served by nginx+Go on Melos). Click a card title → the URL becomes `http://10.190.12.242:8899/items/<id>` and the SSR detail page renders (Chinese title, summary, 阅读原文, ← 返回). Click 返回 → feed. Toggle 日报 / 当前热点 as available. `browser_console_messages` → clean (favicon 404 ok). Screenshot `/Users/didi/Downloads/aihot-live-24x7.png`.

- [ ] **Step 5: Reboot-persistence sanity (optional but recommended).** `ssh melos 'systemctl is-enabled aihot-server'` → `enabled` (survives reboot). And confirm restart works: `ssh melos 'systemctl restart aihot-server && sleep 2 && curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8991/healthz'` → `200`.

- [ ] **Step 6: Update the project runbook pointer.** Append one line to `docs/runbooks/README.md` noting the live URL `http://10.190.12.242:8899/` and that `deploy/README.md` "24/7 托管" has the details. Commit:
```bash
git add docs/runbooks/
git commit -m "docs: note live 24/7 URL"
```

---

## Self-Review

- **Spec coverage:** Delivers 24/7 intranet serving with no Mac/tunnel dependency: cross-compiled server under systemd (Restart=always, enabled for reboot persistence) against the local prod DB; built SPA served by nginx; nginx unifies origin by proxying `/api/` `/items/` `/healthz` to the Go server; a fresh `:8899` server block leaves the existing `:80` untouched; idempotent build+install scripts and a runbook. Deliberately deferred (with a home): HTTPS/TLS (intranet HTTP is the norm here; add a cert + `:443` block later if needed), binding the Go server to `127.0.0.1` only (currently `:8991` all-interfaces — nginx is the front door; a later app.toml tweak can restrict it), nginx gzip/caching tuning (works without; optimize when traffic warrants), and CI-style deploy automation (manual `build → install` is fine for this cadence). The cron pipeline (pulse/gendaily/hotpass) from the prior deploy phase is unchanged and complementary — it feeds the DB this stack serves.
- **Placeholder scan:** No TBD/TODO. Every file is complete; every step is an exact command with expected output. Task 2 is execution with pasted-output checkpoints — the "optional" reboot check is labeled, not a gap.
- **Type/interface consistency:** No new Go code, so no type surface to reconcile. The moving contracts are operational and each is pinned to a verified fact: the server binary is the same `server/.` that booted on Melos in the probe; `http_addr=":8991"` in `conf/app.toml` matches the nginx `proxy_pass 127.0.0.1:8991` and the systemd env; the nginx `root /root/aihot/web` matches where `install-serve-melos.sh` places `out/web`; `WorkingDirectory=/root/aihot` matches where `conf/`+`public/` are shipped so `-c /root/aihot/conf/app.toml` and `./public` resolve. nginx location precedence (`/api/`,`/items/`,`=/healthz` longer/exact vs the `/` catch-all) routes dynamic vs static correctly.

## Follow-on

- **TLS** (`:443` + intranet cert) if the tool needs https.
- **Bind Go to 127.0.0.1** so only nginx fronts it (app.toml `http_addr`).
- **Monitoring/alerting**: nginx access-log based, or a healthcheck cron that pings `:8899` and pushes to the D-Chat bot on failure (pairs with the cron-failure-alert follow-on).
- **Blue/green or versioned web dir** for zero-flicker frontend updates.
