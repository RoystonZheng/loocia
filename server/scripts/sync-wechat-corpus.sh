#!/usr/bin/env bash
# Mirror the Mac-side 公众号 corpus to Melos so the aihot MPCorpusSource can read
# it. Read-only w.r.t. the corpus (never writes back). Run from the Mac (cron).
#
#   ./sync-wechat-corpus.sh [SRC] [DST]
# defaults: /Users/didi/wechat-corpus/  ->  melos:/root/aihot/corpus/wechat/
set -euo pipefail
SRC="${1:-/Users/didi/wechat-corpus/}"
DST="${2:-melos:/root/aihot/corpus/wechat/}"
ssh -o ClearAllForwardings=yes melos "mkdir -p /root/aihot/corpus/wechat"
rsync -az --delete -e "ssh -o ClearAllForwardings=yes" \
  --include='*/' --include='*.md' --exclude='*' \
  "$SRC" "$DST"
echo "synced $SRC -> $DST"
