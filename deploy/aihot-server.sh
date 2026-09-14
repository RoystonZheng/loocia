#!/usr/bin/env bash
# Launch wrapper for the aihot API/SSR server: injects API keys and runtime
# knobs, then execs the server. Keys live only in the wechat-push env file and
# are never logged.
set -euo pipefail
export AIHOT_DATABASE_URL="postgres://aihot:aihot@localhost:5432/aihot"
export AI_TOOL_DATABASE_URL="${AI_TOOL_DATABASE_URL:-$AIHOT_DATABASE_URL}"
export AIHOT_TRANSLATE_RETRY_MODEL="auto-std"
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
exec /root/aihot/bin/server -c /root/aihot/conf/app.toml "$@"
