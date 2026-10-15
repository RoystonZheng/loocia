#!/usr/bin/env bash
# Local cron entry matching production's pulse frequency. A successful pulse
# immediately refreshes the report and hot-topic derived data.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="${AIHOT_LOCAL_LOG_DIR:-$HOME/Library/Logs/aihot}"
LOG_FILE="$LOG_DIR/pulse.log"
LOCK_DIR="${AIHOT_LOCAL_PULSE_LOCK_DIR:-$HOME/Library/Caches/aihot/pulse.lock}"
RUNTIME_BIN_DIR="$SCRIPT_DIR/../bin"
if [[ -x "$RUNTIME_BIN_DIR/pulse" ]]; then
  export AIHOT_LOCAL_BIN_DIR="${AIHOT_LOCAL_BIN_DIR:-$RUNTIME_BIN_DIR}"
fi

mkdir -p "$LOG_DIR" "$(dirname "$LOCK_DIR")"
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  lock_pid="$(cat "$LOCK_DIR/pid" 2>/dev/null || true)"
  if [[ -z "$lock_pid" ]]; then
    sleep 0.1
    lock_pid="$(cat "$LOCK_DIR/pid" 2>/dev/null || true)"
  fi
  if [[ "$lock_pid" =~ ^[0-9]+$ ]] && ! kill -0 "$lock_pid" 2>/dev/null; then
    rm -f "$LOCK_DIR/pid"
    rmdir "$LOCK_DIR" 2>/dev/null || true
    if ! mkdir "$LOCK_DIR" 2>/dev/null; then
      printf '%s pulse skipped: lock was reclaimed by another process\n' \
        "$(date '+%F %T %z')" >>"$LOG_FILE"
      exit 0
    fi
  elif [[ -z "$lock_pid" ]]; then
    lock_mtime="$(stat -f %m "$LOCK_DIR" 2>/dev/null || stat -c %Y "$LOCK_DIR" 2>/dev/null || true)"
    now_epoch="$(date +%s)"
    if [[ "$lock_mtime" =~ ^[0-9]+$ ]] && (( now_epoch - lock_mtime >= 60 )); then
      rmdir "$LOCK_DIR" 2>/dev/null || true
      printf '%s pulse recovered orphaned lock\n' \
        "$(date '+%F %T %z')" >>"$LOG_FILE"
      exit 0
    fi
    printf '%s pulse skipped: lock is initializing\n' \
      "$(date '+%F %T %z')" >>"$LOG_FILE"
    exit 0
  else
    printf '%s pulse skipped: already running pid=%s\n' \
      "$(date '+%F %T %z')" "$lock_pid" >>"$LOG_FILE"
    exit 0
  fi
fi
printf '%s\n' "$$" >"$LOCK_DIR/pid"
trap 'rm -f "$LOCK_DIR/pid"; rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

PULSE_ARGS=()
if [[ -n "${AIHOT_LOCAL_SOURCES_FILE:-}" ]]; then
  PULSE_ARGS=(-sources "$AIHOT_LOCAL_SOURCES_FILE")
fi
if [[ -n "${AIHOT_LOCAL_MP_CORPUS_DIR:-}" ]]; then
  export AIHOT_MP_CORPUS_DIR="$AIHOT_LOCAL_MP_CORPUS_DIR"
fi

{
  printf '%s pulse start\n' "$(date '+%F %T %z')"
  set +e
  if [[ ${#PULSE_ARGS[@]} -gt 0 ]]; then
    "$SCRIPT_DIR/local-aihot-cron-safe.sh" pulse "${PULSE_ARGS[@]}"
  else
    "$SCRIPT_DIR/local-aihot-cron-safe.sh" pulse
  fi
  rc=$?
  set -e
  printf '%s pulse done rc=%s\n' "$(date '+%F %T %z')" "$rc"
  if [[ "$rc" -eq 0 ]]; then
    for derived_unit in gendaily hotpass; do
      printf '%s %s start after pulse\n' "$(date '+%F %T %z')" "$derived_unit"
      set +e
      "$SCRIPT_DIR/local-aihot-unit-trigger.sh" "$derived_unit"
      derived_rc=$?
      set -e
      printf '%s %s done after pulse rc=%s\n' "$(date '+%F %T %z')" "$derived_unit" "$derived_rc"
      if [[ "$derived_rc" -ne 0 ]]; then
        rc="$derived_rc"
      fi
    done
  fi
  exit "$rc"
} >>"$LOG_FILE" 2>&1
