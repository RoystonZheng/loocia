#!/usr/bin/env bash
# Prune rotated hourly framework logs (>2 days) and cap nginx logs at 100M.
set -uo pipefail
LOG=/root/aihot/log
find "$LOG" -maxdepth 1 \( -name "didi.log.*" -o -name "public.log.*" \) -mmin +2880 -delete
for f in "$LOG/nginx-access.log" "$LOG/nginx-error.log"; do
  [ -e "$f" ] || continue
  if [ "$(stat -c%s "$f")" -gt $((100*1024*1024)) ]; then
    cp "$f" "$f.1" && : > "$f"
  fi
done
