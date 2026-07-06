#!/usr/bin/env bash
# Cron wrapper: env + LLM key + exec one pipeline unit, logging to /root/aihot/log.
#   aihot-cron.sh pulse|gendaily|hotpass [extra args...]
set -euo pipefail
UNIT="$1"; shift || true

export AIHOT_DATABASE_URL="postgres://aihot:aihot@localhost:5432/aihot"
# The LLM key lives in the wechat-push env file on this host.
export AIHOT_LLM_API_KEY="$(grep '^LLM_API_KEY' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"

LOG="/root/aihot/log/${UNIT}.log"
{
  echo "--- $(date -u '+%F %T') UTC ${UNIT} start"
  "/root/aihot/bin/${UNIT}" "$@" 2>&1
  echo "--- $(date -u '+%F %T') UTC ${UNIT} done rc=$?"
} >> "$LOG" 2>&1
