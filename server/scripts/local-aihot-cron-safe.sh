#!/usr/bin/env bash
# Run AIHOT pipeline units against the local production clone only.
#
# Usage:
#   ./scripts/local-aihot-cron-safe.sh pulse
#   ./scripts/local-aihot-cron-safe.sh gendaily
#   ./scripts/local-aihot-cron-safe.sh hotpass
#   ./scripts/local-aihot-cron-safe.sh discovertools scheduled -actor local
#
# The script refuses any DSN that does not target aihot_local_prod_clone, so it
# cannot accidentally write to Melos/production. Optional local credentials
# come from the process environment or ~/Library/aihot-local/local.env. This
# script never SSHes to production.
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
LOCAL_DB="aihot_local_prod_clone"

# cron and launchd do not inherit the interactive shell environment. Keep
# credentials local and explicit instead of reaching into the production host.
LOCAL_ENV_FILE="${AIHOT_LOCAL_ENV_FILE:-$HOME/Library/aihot-local/local.env}"
if [[ -f "$LOCAL_ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  . "$LOCAL_ENV_FILE"
  set +a
fi

export PATH="${AIHOT_LOCAL_PATH:-/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin}:$PATH"
export AIHOT_DATABASE_URL="${AIHOT_DATABASE_URL:-postgres://didi@localhost:5432/${LOCAL_DB}?sslmode=disable}"
export AI_TOOL_DATABASE_URL="${AI_TOOL_DATABASE_URL:-$AIHOT_DATABASE_URL}"
export AIHOT_LLM_MODEL="deepseek-v4-flash"
export AIHOT_TRANSLATE_MODEL="deepseek-v4-flash"
export AIHOT_TRANSLATE_RETRY_MODEL="deepseek-v4-flash"
export AIHOT_TERMS_MODEL="deepseek-v4-flash"

dsn_uses_local_clone() {
  local dsn="$1"
  local database="${dsn##*/}"
  database="${database%%\?*}"
  [[ "$database" == "$LOCAL_DB" ]]
}

if ! dsn_uses_local_clone "$AIHOT_DATABASE_URL"; then
    echo "refusing to run: AIHOT_DATABASE_URL must point at aihot_local_prod_clone" >&2
    echo "current AIHOT_DATABASE_URL=$AIHOT_DATABASE_URL" >&2
    exit 3
fi

if ! dsn_uses_local_clone "$AI_TOOL_DATABASE_URL"; then
    echo "refusing to run: AI_TOOL_DATABASE_URL must point at aihot_local_prod_clone" >&2
    echo "current AI_TOOL_DATABASE_URL=$AI_TOOL_DATABASE_URL" >&2
    exit 3
fi

GO_BIN="${AIHOT_GO_BIN:-$(command -v go || true)}"
if [[ -z "$GO_BIN" || ! -x "$GO_BIN" ]]; then
  echo "go executable not found; set AIHOT_GO_BIN or fix PATH" >&2
  exit 5
fi

if [[ "$UNIT" == "pulse" && -z "${AIHOT_LLM_API_KEY:-}" ]]; then
  # Keep source collection testable without pretending that enrichment ran.
  # cmd/pulse will persist raw_items and leave them pending for the next full
  # pulse after a local LLM credential becomes available.
  export AIHOT_PULSE_INGEST_ONLY=1
  echo "AIHOT_LLM_API_KEY unavailable; running pulse in local ingest-only mode" >&2
elif [[ "$UNIT" == "hotpass" ]] && [[ -z "${AIHOT_LLM_API_KEY:-}" ]]; then
  echo "AIHOT_LLM_API_KEY unavailable; set it in $LOCAL_ENV_FILE or the environment before running $UNIT" >&2
  exit 4
fi

cd "$SERVER_DIR"
if [[ -n "${AIHOT_LOCAL_BIN_DIR:-}" ]]; then
  UNIT_BIN="$AIHOT_LOCAL_BIN_DIR/$UNIT"
  if [[ ! -x "$UNIT_BIN" ]]; then
    echo "local runtime binary not found: $UNIT_BIN" >&2
    exit 5
  fi
  exec "$UNIT_BIN" "$@"
fi
"$GO_BIN" run "./cmd/${UNIT}" "$@"
