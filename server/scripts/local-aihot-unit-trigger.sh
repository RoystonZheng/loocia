#!/usr/bin/env bash
# Local runner for report and hotpass jobs invoked after pulse or manually.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNIT="${1:-}"
case "$UNIT" in
  gendaily|hotpass) ;;
  *)
    echo "usage: $0 gendaily|hotpass" >&2
    exit 2
    ;;
esac

LOG_DIR="${AIHOT_LOCAL_LOG_DIR:-$HOME/Library/Logs/aihot}"
LOG_FILE="$LOG_DIR/$UNIT.log"
if [[ "$UNIT" == "gendaily" ]]; then
  LOCK_DIR="${AIHOT_LOCAL_GENDAILY_LOCK_DIR:-$HOME/Library/Caches/aihot/gendaily.lock}"
else
  LOCK_DIR="${AIHOT_LOCAL_HOTPASS_LOCK_DIR:-$HOME/Library/Caches/aihot/hotpass.lock}"
fi
RUNTIME_BIN_DIR="$SCRIPT_DIR/../bin"
if [[ -x "$RUNTIME_BIN_DIR/$UNIT" ]]; then
  export AIHOT_LOCAL_BIN_DIR="${AIHOT_LOCAL_BIN_DIR:-$RUNTIME_BIN_DIR}"
fi

mkdir -p "$LOG_DIR" "$(dirname "$LOCK_DIR")"
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  printf '%s %s skipped: already running\n' "$(date '+%F %T %z')" "$UNIT" >>"$LOG_FILE"
  exit 0
fi
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

{
  printf '%s %s start\n' "$(date '+%F %T %z')" "$UNIT"
  set +e
  "$SCRIPT_DIR/local-aihot-cron-safe.sh" "$UNIT"
  rc=$?
  set -e
  printf '%s %s done rc=%s\n' "$(date '+%F %T %z')" "$UNIT" "$rc"
  exit "$rc"
} >>"$LOG_FILE" 2>&1
