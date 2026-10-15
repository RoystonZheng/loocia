#!/usr/bin/env bash
# Local cron entry matching the post-update production discovertools trigger.
# The every-minute tick only checks due daily/weekly configurations.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMAND="${1:-scheduled}"
case "$COMMAND" in
  scheduled|snapshot) ;;
  *)
    echo "unsupported discovertools command: $COMMAND" >&2
    exit 2
    ;;
esac
LOG_DIR="${AIHOT_LOCAL_LOG_DIR:-$HOME/Library/Logs/aihot}"
LOG_FILE="$LOG_DIR/discovertools.log"
LOCK_DIR="${AIHOT_LOCAL_LOCK_DIR:-$HOME/Library/Caches/aihot/discovertools.lock}"
RUNTIME_BIN_DIR="$SCRIPT_DIR/../bin"
if [[ -x "$RUNTIME_BIN_DIR/discovertools" ]]; then
  export AIHOT_LOCAL_BIN_DIR="${AIHOT_LOCAL_BIN_DIR:-$RUNTIME_BIN_DIR}"
fi

mkdir -p "$LOG_DIR"
mkdir -p "$(dirname "$LOCK_DIR")"
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  lock_pid="$(cat "$LOCK_DIR/pid" 2>/dev/null || true)"
  if [[ -z "$lock_pid" ]]; then
    # The owner writes pid immediately after mkdir, but give that tiny window
    # a chance to close before treating the lock as orphaned.
    sleep 0.1
    lock_pid="$(cat "$LOCK_DIR/pid" 2>/dev/null || true)"
  fi
  if [[ "$lock_pid" =~ ^[0-9]+$ ]] && ! kill -0 "$lock_pid" 2>/dev/null; then
    rm -f "$LOCK_DIR/pid"
    rmdir "$LOCK_DIR" 2>/dev/null || true
    if ! mkdir "$LOCK_DIR" 2>/dev/null; then
      printf '%s discovertools %s skipped: lock was reclaimed by another process\n' \
        "$(date '+%F %T %z')" "$COMMAND" >>"$LOG_FILE"
      exit 0
    fi
  elif [[ -z "$lock_pid" ]]; then
    lock_mtime="$(stat -f %m "$LOCK_DIR" 2>/dev/null || stat -c %Y "$LOCK_DIR" 2>/dev/null || true)"
    now_epoch="$(date +%s)"
    if [[ "$lock_mtime" =~ ^[0-9]+$ ]] && (( now_epoch - lock_mtime >= 60 )); then
      rmdir "$LOCK_DIR" 2>/dev/null || true
      printf '%s discovertools %s recovered orphaned lock\n' \
        "$(date '+%F %T %z')" "$COMMAND" >>"$LOG_FILE"
      exit 0
    fi
    printf '%s discovertools %s skipped: lock is initializing\n' \
      "$(date '+%F %T %z')" "$COMMAND" >>"$LOG_FILE"
    exit 0
  else
    printf '%s discovertools %s skipped: already running pid=%s\n' \
      "$(date '+%F %T %z')" "$COMMAND" "$lock_pid" >>"$LOG_FILE"
    exit 0
  fi
fi
printf '%s\n' "$$" >"$LOCK_DIR/pid"
trap 'rm -f "$LOCK_DIR/pid"; rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

{
  printf '%s discovertools %s start\n' "$(date '+%F %T %z')" "$COMMAND"
  set +e
  if [[ "$COMMAND" == "scheduled" ]]; then
    "$SCRIPT_DIR/local-aihot-cron-safe.sh" discovertools scheduled -actor local-cron
  else
    "$SCRIPT_DIR/local-aihot-cron-safe.sh" discovertools snapshot
  fi
  rc=$?
  set -e
  printf '%s discovertools %s done rc=%s\n' "$(date '+%F %T %z')" "$COMMAND" "$rc"
  exit "$rc"
} >>"$LOG_FILE" 2>&1
