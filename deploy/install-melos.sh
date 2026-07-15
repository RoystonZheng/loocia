#!/usr/bin/env bash
# Install/refresh the pipeline on Melos: binaries + wrapper + crontab.
set -euo pipefail
cd "$(dirname "$0")"
HOST="${1:-melos}"

[ -x out/pulse ] || { echo "run build-linux.sh first"; exit 1; }

ssh "$HOST" 'mkdir -p /root/aihot/bin /root/aihot/etc /root/aihot/log'
scp out/pulse out/gendaily out/hotpass aihot-cron.sh aihot-backup.sh aihot-logclean.sh "$HOST:/root/aihot/bin/"
ssh "$HOST" 'chmod +x /root/aihot/bin/*'

# Crontab: keep other entries, replace the aihot block idempotently.
# importmp is a backlog catch-up over the synced corpus (steady-state mp ingest
# is pulse's job); it must point at the canonical corpus dir explicitly because
# the binary reads -corpus, not AIHOT_MP_CORPUS_DIR.
ssh "$HOST" '
  (crontab -l 2>/dev/null | sed "/# aihot-begin/,/# aihot-end/d"
   echo "# aihot-begin"
   echo "25 * * * * /root/aihot/bin/aihot-cron.sh importmp -corpus /root/wechat-corpus"
   echo "*/30 * * * * /root/aihot/bin/aihot-cron.sh pulse"
   echo "10 * * * * /root/aihot/bin/aihot-cron.sh gendaily"
   echo "20 * * * * /root/aihot/bin/aihot-cron.sh hotpass"
   echo "45 3 * * * /root/aihot/bin/aihot-logclean.sh"
   echo "50 3 * * * /root/aihot/bin/aihot-backup.sh"
   echo "# aihot-end") | crontab -
  crontab -l | sed -n "/# aihot-begin/,/# aihot-end/p"
'
echo "installed."
