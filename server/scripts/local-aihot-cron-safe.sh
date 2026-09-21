#!/usr/bin/env bash
# Run AIHOT pipeline units against the local production clone only.
#
# Usage:
#   ./scripts/local-aihot-cron-safe.sh pulse
#   ./scripts/local-aihot-cron-safe.sh gendaily
#   ./scripts/local-aihot-cron-safe.sh hotpass
#   ./scripts/local-aihot-cron-safe.sh discovertools weekly -actor local
#
# The script refuses any DSN that does not target aihot_local_prod_clone, so it
# cannot accidentally write to Melos/production. Secrets are read from Melos at
# runtime and are never written to disk by this script.
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 pulse|gendaily|hotpass|discovertools [args...]" >&2
  exit 2
fi

UNIT="$1"
shift || true

case "$UNIT" in
  pulse|gendaily|hotpass|discovertools) ;;
  *)
    echo "unsupported unit: $UNIT" >&2
    exit 2
    ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
LOCAL_DB="${AIHOT_LOCAL_DATABASE:-aihot_local_prod_clone}"

export AIHOT_DATABASE_URL="${AIHOT_DATABASE_URL:-postgres://didi@localhost:5432/${LOCAL_DB}?sslmode=disable}"
export AI_TOOL_DATABASE_URL="${AI_TOOL_DATABASE_URL:-$AIHOT_DATABASE_URL}"

case "$AIHOT_DATABASE_URL" in
  *"aihot_local_prod_clone"*) ;;
  *)
    echo "refusing to run: AIHOT_DATABASE_URL must point at aihot_local_prod_clone" >&2
    echo "current AIHOT_DATABASE_URL=$AIHOT_DATABASE_URL" >&2
    exit 3
    ;;
esac

read_remote_secret() {
  local key="$1"
  ssh root@10.190.12.242 "grep '^${key}=' /root/wechat-push/.env 2>/dev/null | head -n1 | cut -d= -f2- | tr -d '\"' | tr -d \"'\" | tr -d '\r'"
}

if [[ "$UNIT" == "pulse" || "$UNIT" == "gendaily" || "$UNIT" == "hotpass" ]]; then
  export AIHOT_LLM_API_KEY="${AIHOT_LLM_API_KEY:-$(read_remote_secret LLM_API_KEY)}"
  if [[ -z "${AIHOT_LLM_API_KEY:-}" ]]; then
    echo "AIHOT_LLM_API_KEY unavailable; cannot run $UNIT" >&2
    exit 4
  fi
fi

if [[ "$UNIT" == "discovertools" ]]; then
  export AI_TOOL_GITHUB_TOKEN="${AI_TOOL_GITHUB_TOKEN:-$(read_remote_secret AI_TOOL_GITHUB_TOKEN)}"
  if [[ -z "${AI_TOOL_GITHUB_TOKEN:-}" ]]; then
    echo "AI_TOOL_GITHUB_TOKEN unavailable; cannot run discovertools" >&2
    exit 4
  fi
fi

cd "$SERVER_DIR"
go run "./cmd/${UNIT}" "$@"
