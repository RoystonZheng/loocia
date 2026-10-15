#!/usr/bin/env bash
# Keep the local HTTP backend running against the local production clone.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
LOCAL_DB="aihot_local_prod_clone"
LOCAL_ENV_FILE="${AIHOT_LOCAL_ENV_FILE:-$HOME/Library/aihot-local/local.env}"

# launchd does not inherit the interactive shell. Load the local-only LLM
# credential here so the API enables the same LLM-backed features as cron.
if [[ -f "$LOCAL_ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  . "$LOCAL_ENV_FILE"
  set +a
fi

export PATH="${AIHOT_LOCAL_PATH:-/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin}:$PATH"
export AIHOT_HTTP_ADDR="${AIHOT_HTTP_ADDR:-127.0.0.1:8991}"
export AIHOT_DATABASE_URL="${AIHOT_DATABASE_URL:-postgres://didi@localhost:5432/${LOCAL_DB}?sslmode=disable}"
export AI_TOOL_DATABASE_URL="${AI_TOOL_DATABASE_URL:-$AIHOT_DATABASE_URL}"
export AI_TOOL_WEB_API_PROXY="${AI_TOOL_WEB_API_PROXY:-http://127.0.0.1:8991}"
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
    echo "refusing to start: AIHOT_DATABASE_URL must point at aihot_local_prod_clone" >&2
    exit 3
fi

if ! dsn_uses_local_clone "$AI_TOOL_DATABASE_URL"; then
    echo "refusing to start: AI_TOOL_DATABASE_URL must point at aihot_local_prod_clone" >&2
    exit 3
fi

GO_BIN="${AIHOT_GO_BIN:-$(command -v go || true)}"
SERVER_BIN="${AIHOT_LOCAL_SERVER_BIN:-}"
if [[ -n "$SERVER_BIN" && ! -x "$SERVER_BIN" ]]; then
  echo "local runtime server binary not found: $SERVER_BIN" >&2
  exit 5
fi
if [[ -z "$SERVER_BIN" && ( -z "$GO_BIN" || ! -x "$GO_BIN" ) ]]; then
  echo "go executable not found; set AIHOT_GO_BIN or fix PATH" >&2
  exit 5
fi

if [[ -n "$SERVER_BIN" ]]; then
  exec "$SERVER_BIN"
fi

cd "$SERVER_DIR"
exec "$GO_BIN" run ./cmd/webserver
