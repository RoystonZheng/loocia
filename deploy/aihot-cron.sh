#!/usr/bin/env bash
# Cron wrapper: env + API keys + exec one pipeline unit, logging to /root/aihot/log.
#   aihot-cron.sh pulse|gendaily|hotpass|discovertools [extra args...]
set -euo pipefail
if [[ $# -lt 1 ]]; then
  echo "usage: $0 pulse|gendaily|hotpass|discovertools [args...]" >&2
  exit 2
fi
UNIT="$1"; shift || true

export AIHOT_DATABASE_URL="postgres://aihot:aihot@localhost:5432/aihot"
export AI_TOOL_DATABASE_URL="${AI_TOOL_DATABASE_URL:-$AIHOT_DATABASE_URL}"
export AIHOT_MP_CORPUS_DIR="/root/wechat-corpus"
export AIHOT_LLM_MODEL="deepseek-v4-flash"
export AIHOT_TRANSLATE_MODEL="deepseek-v4-flash"
export AIHOT_TRANSLATE_RETRY_MODEL="deepseek-v4-flash"
export AIHOT_TERMS_MODEL="deepseek-v4-flash"
# Source-only egress proxy. Keep this separate from the internal LLM proxy.
if grep -q '^AIHOT_SOURCE_HTTP_PROXY=' /root/wechat-push/.env; then
  export AIHOT_SOURCE_HTTP_PROXY="$(grep '^AIHOT_SOURCE_HTTP_PROXY=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AIHOT_SOURCES_FILE=' /root/wechat-push/.env; then
  export AIHOT_SOURCES_FILE="$(grep '^AIHOT_SOURCES_FILE=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AIHOT_MP_OFFICIAL_ACCOUNTS=' /root/wechat-push/.env; then
  export AIHOT_MP_OFFICIAL_ACCOUNTS="$(grep '^AIHOT_MP_OFFICIAL_ACCOUNTS=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
# The LLM key lives in the wechat-push env file on this host.
export AIHOT_LLM_API_KEY="$(grep '^LLM_API_KEY' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
if grep -q '^AI_TOOL_GITHUB_TOKEN=' /root/wechat-push/.env; then
  export AI_TOOL_GITHUB_TOKEN="$(grep '^AI_TOOL_GITHUB_TOKEN=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AI_TOOL_GITHUB_TOKENS=' /root/wechat-push/.env; then
  export AI_TOOL_GITHUB_TOKENS="$(grep '^AI_TOOL_GITHUB_TOKENS=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AI_TOOL_GITHUB_BASE_URL=' /root/wechat-push/.env; then
  export AI_TOOL_GITHUB_BASE_URL="$(grep '^AI_TOOL_GITHUB_BASE_URL=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AI_TOOL_GITHUB_MAX_PAGES=' /root/wechat-push/.env; then
  export AI_TOOL_GITHUB_MAX_PAGES="$(grep '^AI_TOOL_GITHUB_MAX_PAGES=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AI_TOOL_GITHUB_PER_PAGE=' /root/wechat-push/.env; then
  export AI_TOOL_GITHUB_PER_PAGE="$(grep '^AI_TOOL_GITHUB_PER_PAGE=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AI_TOOL_GITHUB_REQUEST_INTERVAL_MS=' /root/wechat-push/.env; then
  export AI_TOOL_GITHUB_REQUEST_INTERVAL_MS="$(grep '^AI_TOOL_GITHUB_REQUEST_INTERVAL_MS=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AI_TOOL_STAR_SNAPSHOT_LIMIT=' /root/wechat-push/.env; then
  export AI_TOOL_STAR_SNAPSHOT_LIMIT="$(grep '^AI_TOOL_STAR_SNAPSHOT_LIMIT=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if [[ "$UNIT" == "discovertools" && -z "${AI_TOOL_GITHUB_TOKEN:-}" && -z "${AI_TOOL_GITHUB_TOKENS:-}" ]]; then
  echo "AI_TOOL_GITHUB_TOKEN(S) not set"
  exit 1
fi
if [[ "$UNIT" == "pulse" && $# -eq 0 && -n "${AIHOT_SOURCES_FILE:-}" ]]; then
  set -- -sources "$AIHOT_SOURCES_FILE"
fi

LOG="/root/aihot/log/${UNIT}.log"
exec 9>"/root/aihot/log/${UNIT}.lock"
if ! flock -n 9; then
  echo "--- $(date -u '+%F %T') UTC ${UNIT} skipped: already running" >> "$LOG"
  exit 0
fi
{
  echo "--- $(date -u '+%F %T') UTC ${UNIT} start"
  set +e
  "/root/aihot/bin/${UNIT}" "$@"
  rc=$?
  set -e
  echo "--- $(date -u '+%F %T') UTC ${UNIT} done rc=$rc"
  if [[ "$UNIT" == "pulse" && "$rc" -eq 0 ]]; then
    for derived_unit in gendaily hotpass; do
      echo "--- $(date -u '+%F %T') UTC ${derived_unit} start after pulse"
      set +e
      "/root/aihot/bin/aihot-cron.sh" "$derived_unit"
      derived_rc=$?
      set -e
      echo "--- $(date -u '+%F %T') UTC ${derived_unit} done after pulse rc=$derived_rc"
      if [[ "$derived_rc" -ne 0 ]]; then
        rc="$derived_rc"
      fi
    done
  fi
  exit "$rc"
} >> "$LOG" 2>&1
