#!/usr/bin/env bash
# Install/refresh the pipeline on Melos: binaries + wrapper + crontab.
set -euo pipefail
cd "$(dirname "$0")"
HOST="${1:-melos}"

[ -x out/pulse ] || { echo "run build-linux.sh first"; exit 1; }

ssh "$HOST" 'mkdir -p /root/aihot/bin /root/aihot/etc /root/aihot/log'
scp out/pulse out/gendaily out/hotpass aihot-cron.sh "$HOST:/root/aihot/bin/"
ssh "$HOST" 'chmod +x /root/aihot/bin/*'

# Crontab: keep other entries, replace the aihot block idempotently.
ssh "$HOST" '
  (crontab -l 2>/dev/null | sed "/# aihot-begin/,/# aihot-end/d"
   echo "# aihot-begin"
   echo "*/30 * * * * /root/aihot/bin/aihot-cron.sh pulse"
   echo "10 * * * * /root/aihot/bin/aihot-cron.sh gendaily"
   echo "20 * * * * /root/aihot/bin/aihot-cron.sh hotpass"
   echo "# aihot-end") | crontab -
  crontab -l | sed -n "/# aihot-begin/,/# aihot-end/p"
'
echo "installed."
