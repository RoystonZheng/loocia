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
