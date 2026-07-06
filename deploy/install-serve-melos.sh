#!/usr/bin/env bash
# Install/refresh the 24/7 serving stack on Melos: server binary + conf + public
# + web bundle + supervisor program + nginx server block. Idempotent.
# Melos uses supervisord (PID 1), not systemd; nginx is reloaded via `nginx -s reload`.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/deploy"
HOST="${1:-melos}"

[ -x out/server ] || { echo "run build-linux.sh first (out/server missing)"; exit 1; }
[ -d out/web ]    || { echo "run build-linux.sh first (out/web missing)"; exit 1; }

echo "== dirs =="
# Web root lives under /var/www (world-readable) — nginx runs as www-data and
# cannot traverse /root (mode 700). Server bin/conf/log stay under /root/aihot.
ssh "$HOST" 'mkdir -p /root/aihot/bin /root/aihot/conf /root/aihot/log /var/www/aihot'

echo "== server binary + conf + public =="
# Stop a running server first — can't overwrite a busy binary (ETXTBSY).
ssh "$HOST" 'supervisorctl stop aihot-server 2>/dev/null; true'
scp out/server "$HOST:/root/aihot/bin/server"
ssh "$HOST" 'chmod +x /root/aihot/bin/server'
scp -r "$ROOT/server/conf/." "$HOST:/root/aihot/conf/"
# public/ is optional (SSE demo assets); ship if present
if [ -d "$ROOT/server/public" ]; then scp -r "$ROOT/server/public" "$HOST:/root/aihot/"; fi

echo "== web bundle =="
ssh "$HOST" 'rm -rf /var/www/aihot && mkdir -p /var/www/aihot'
scp -r out/web/. "$HOST:/var/www/aihot/"
ssh "$HOST" 'chmod -R a+rX /var/www/aihot'

echo "== supervisor program =="
scp aihot-server.supervisor.conf "$HOST:/etc/supervisor/conf.d/aihot-server.conf"
# reread+update registers/refreshes the program; restart picks up a new binary.
ssh "$HOST" 'supervisorctl reread && supervisorctl update && supervisorctl restart aihot-server 2>/dev/null || supervisorctl start aihot-server; sleep 3; supervisorctl status aihot-server'

echo "== nginx server block =="
scp aihot.nginx.conf "$HOST:/etc/nginx/conf.d/aihot.conf"
ssh "$HOST" 'nginx -t && nginx -s reload'

echo "== verify on host =="
ssh "$HOST" '
  curl -s -o /dev/null -w "server :8991 healthz=%{http_code}\n" http://127.0.0.1:8991/healthz
  curl -s -o /dev/null -w "nginx  :8899 spa=%{http_code}\n"    http://127.0.0.1:8899/
  curl -s -o /dev/null -w "nginx  :8899 api=%{http_code}\n"    "http://127.0.0.1:8899/api/public/items?take=1"
'
echo "installed. → http://10.190.12.242:8899/"
