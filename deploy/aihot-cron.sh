#!/usr/bin/env bash
# Cron wrapper: env + API keys + exec one pipeline unit, logging to /root/aihot/log.
#   aihot-cron.sh pulse|gendaily|hotpass|discovertools [extra args...]
set -euo pipefail
UNIT="$1"; shift || true

export AIHOT_DATABASE_URL="postgres://aihot:aihot@localhost:5432/aihot"
export AI_TOOL_DATABASE_URL="${AI_TOOL_DATABASE_URL:-$AIHOT_DATABASE_URL}"
export AIHOT_MP_CORPUS_DIR="/root/wechat-corpus"
export AIHOT_TRANSLATE_MODEL="deepseek-v4-flash"
# Source-only egress proxy. Keep this separate from the internal LLM proxy.
if grep -q '^AIHOT_SOURCE_HTTP_PROXY=' /root/wechat-push/.env; then
  export AIHOT_SOURCE_HTTP_PROXY="$(grep '^AIHOT_SOURCE_HTTP_PROXY=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
# The LLM key lives in the wechat-push env file on this host.
export AIHOT_LLM_API_KEY="$(grep '^LLM_API_KEY' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
if grep -q '^AI_TOOL_GITHUB_TOKEN=' /root/wechat-push/.env; then
  export AI_TOOL_GITHUB_TOKEN="$(grep '^AI_TOOL_GITHUB_TOKEN=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AI_TOOL_GITHUB_MAX_PAGES=' /root/wechat-push/.env; then
  export AI_TOOL_GITHUB_MAX_PAGES="$(grep '^AI_TOOL_GITHUB_MAX_PAGES=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AI_TOOL_GITHUB_REQUEST_INTERVAL_MS=' /root/wechat-push/.env; then
  export AI_TOOL_GITHUB_REQUEST_INTERVAL_MS="$(grep '^AI_TOOL_GITHUB_REQUEST_INTERVAL_MS=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if grep -q '^AI_TOOL_STAR_SNAPSHOT_LIMIT=' /root/wechat-push/.env; then
  export AI_TOOL_STAR_SNAPSHOT_LIMIT="$(grep '^AI_TOOL_STAR_SNAPSHOT_LIMIT=' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
fi
if [[ "$UNIT" == "discovertools" && -z "${AI_TOOL_GITHUB_TOKEN:-}" ]]; then
  echo "AI_TOOL_GITHUB_TOKEN not set"
  exit 1
fi
if [[ "$UNIT" == "pulse" && $# -eq 0 && -n "${AIHOT_SOURCES_FILE:-}" ]]; then
  set -- -sources "$AIHOT_SOURCES_FILE"
fi

LOG="/root/aihot/log/${UNIT}.log"
{
  echo "--- $(date -u '+%F %T') UTC ${UNIT} start"
  "/root/aihot/bin/${UNIT}" "$@" 2>&1
  echo "--- $(date -u '+%F %T') UTC ${UNIT} done rc=$?"
} >> "$LOG" 2>&1
