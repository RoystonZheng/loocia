#!/usr/bin/env bash
# Launch wrapper for the aihot API/SSR server: injects the LLM key (for the
# POST /items/{id}/retranslate endpoint) + retry model, then execs the server.
# The key lives only in the wechat-push env file and is never logged.
set -euo pipefail
export AIHOT_DATABASE_URL="postgres://aihot:aihot@localhost:5432/aihot"
export AIHOT_TRANSLATE_RETRY_MODEL="auto-std"
export AIHOT_LLM_API_KEY="$(grep '^LLM_API_KEY' /root/wechat-push/.env | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d '\r')"
exec /root/aihot/bin/server -c /root/aihot/conf/app.toml "$@"
