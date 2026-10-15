#!/usr/bin/env bash
# Install the local backend keeper and the post-update production-like tool
# scheduler. This only writes the user's LaunchAgent and crontab.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SERVER_SCRIPT="$SCRIPT_DIR/local-aihot-server.sh"
CRON_SAFE_SCRIPT="$SCRIPT_DIR/local-aihot-cron-safe.sh"
PULSE_TRIGGER="$SCRIPT_DIR/local-aihot-pulse-trigger.sh"
TRIGGER_SCRIPT="$SCRIPT_DIR/local-aihot-discovertools-trigger.sh"
UNIT_TRIGGER="$SCRIPT_DIR/local-aihot-unit-trigger.sh"
LABEL="com.aihot.local-server"
UID_NUMBER="$(id -u)"
PLIST_DIR="$HOME/Library/LaunchAgents"
PLIST="$PLIST_DIR/$LABEL.plist"
LOG_DIR="${AIHOT_LOCAL_LOG_DIR:-$HOME/Library/Logs/aihot}"
RUNTIME_DIR="${AIHOT_LOCAL_RUNTIME_DIR:-$HOME/Library/aihot-local}"
RUNTIME_SCRIPT_DIR="$RUNTIME_DIR/scripts"
RUNTIME_BIN_DIR="$RUNTIME_DIR/bin"
RUNTIME_SERVER_SCRIPT="$RUNTIME_SCRIPT_DIR/local-aihot-server.sh"
RUNTIME_CRON_SAFE_SCRIPT="$RUNTIME_SCRIPT_DIR/local-aihot-cron-safe.sh"
RUNTIME_PULSE_TRIGGER="$RUNTIME_SCRIPT_DIR/local-aihot-pulse-trigger.sh"
RUNTIME_TRIGGER_SCRIPT="$RUNTIME_SCRIPT_DIR/local-aihot-discovertools-trigger.sh"
RUNTIME_UNIT_TRIGGER="$RUNTIME_SCRIPT_DIR/local-aihot-unit-trigger.sh"
LOCAL_DB="aihot_local_prod_clone"
GO_BIN="${AIHOT_GO_BIN:-$(command -v go || true)}"

if [[ -z "$GO_BIN" || ! -x "$GO_BIN" ]]; then
  echo "go executable not found; set AIHOT_GO_BIN or fix PATH" >&2
  exit 5
fi
if ! command -v launchctl >/dev/null 2>&1; then
  echo "launchctl is required on macOS" >&2
  exit 6
fi
if ! command -v crontab >/dev/null 2>&1; then
  echo "crontab is required to install the local scheduler" >&2
  exit 6
fi

mkdir -p "$PLIST_DIR" "$LOG_DIR"
mkdir -p "$RUNTIME_SCRIPT_DIR" "$RUNTIME_BIN_DIR"

# launchd and cron may not have permission to read a repository below
# ~/Documents. Build and stage the two long-lived entrypoints under ~/Library,
# while keeping the source of truth in this repository.
cd "$SERVER_DIR"
GOTOOLCHAIN=local "$GO_BIN" build -o "$RUNTIME_BIN_DIR/server" ./cmd/webserver
GOTOOLCHAIN=local "$GO_BIN" build -o "$RUNTIME_BIN_DIR/pulse" ./cmd/pulse
GOTOOLCHAIN=local "$GO_BIN" build -o "$RUNTIME_BIN_DIR/gendaily" ./cmd/gendaily
GOTOOLCHAIN=local "$GO_BIN" build -o "$RUNTIME_BIN_DIR/hotpass" ./cmd/hotpass
GOTOOLCHAIN=local "$GO_BIN" build -o "$RUNTIME_BIN_DIR/discovertools" ./cmd/discovertools
cp "$SERVER_SCRIPT" "$RUNTIME_SERVER_SCRIPT"
cp "$CRON_SAFE_SCRIPT" "$RUNTIME_CRON_SAFE_SCRIPT"
cp "$PULSE_TRIGGER" "$RUNTIME_PULSE_TRIGGER"
cp "$TRIGGER_SCRIPT" "$RUNTIME_TRIGGER_SCRIPT"
cp "$UNIT_TRIGGER" "$RUNTIME_UNIT_TRIGGER"
chmod +x "$RUNTIME_SERVER_SCRIPT" "$RUNTIME_CRON_SAFE_SCRIPT" "$RUNTIME_PULSE_TRIGGER" \
  "$RUNTIME_TRIGGER_SCRIPT" "$RUNTIME_UNIT_TRIGGER" "$RUNTIME_BIN_DIR/server" \
  "$RUNTIME_BIN_DIR/pulse" "$RUNTIME_BIN_DIR/gendaily" "$RUNTIME_BIN_DIR/hotpass" \
  "$RUNTIME_BIN_DIR/discovertools"

# Stop a previous managed instance before replacing its plist. Do not kill an
# unrelated process that happens to own the port.
launchctl bootout "gui/$UID_NUMBER/$LABEL" >/dev/null 2>&1 || true
sleep 1
if command -v lsof >/dev/null 2>&1; then
  PORT_PID="$(lsof -nP -iTCP:8991 -sTCP:LISTEN -t | head -n1 || true)"
  if [[ -n "$PORT_PID" ]]; then
    echo "port 8991 is already in use by PID $PORT_PID; stop it before installing" >&2
    exit 7
  fi
fi

cat >"$PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>$RUNTIME_SERVER_SCRIPT</string>
  </array>
  <key>WorkingDirectory</key>
  <string>$RUNTIME_DIR</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
    <key>AIHOT_GO_BIN</key>
    <string>$GO_BIN</string>
    <key>AIHOT_LOCAL_SERVER_BIN</key>
    <string>$RUNTIME_BIN_DIR/server</string>
    <key>AIHOT_LOCAL_BIN_DIR</key>
    <string>$RUNTIME_BIN_DIR</string>
    <key>AIHOT_HTTP_ADDR</key>
    <string>127.0.0.1:8991</string>
    <key>AIHOT_DATABASE_URL</key>
    <string>postgres://didi@localhost:5432/$LOCAL_DB?sslmode=disable</string>
    <key>AI_TOOL_DATABASE_URL</key>
    <string>postgres://didi@localhost:5432/$LOCAL_DB?sslmode=disable</string>
    <key>AI_TOOL_WEB_API_PROXY</key>
    <string>http://127.0.0.1:8991</string>
    <key>AIHOT_LLM_MODEL</key>
    <string>deepseek-v4-flash</string>
    <key>AIHOT_TRANSLATE_MODEL</key>
    <string>deepseek-v4-flash</string>
    <key>AIHOT_TRANSLATE_RETRY_MODEL</key>
    <string>deepseek-v4-flash</string>
    <key>AIHOT_TERMS_MODEL</key>
    <string>deepseek-v4-flash</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProcessType</key>
  <string>Interactive</string>
  <key>ThrottleInterval</key>
  <integer>5</integer>
  <key>StandardOutPath</key>
  <string>$LOG_DIR/server.log</string>
  <key>StandardErrorPath</key>
  <string>$LOG_DIR/server.log</string>
</dict>
</plist>
PLIST

launchctl bootstrap "gui/$UID_NUMBER" "$PLIST"
launchctl kickstart -k "gui/$UID_NUMBER/$LABEL"

CRON_BEGIN="# aihot-local-tools-begin"
CRON_END="# aihot-local-tools-end"
CURRENT_CRON="$(crontab -l 2>/dev/null || true)"
CLEAN_CRON="$(printf '%s\n' "$CURRENT_CRON" | sed "/$CRON_BEGIN/,/$CRON_END/d")"
{
  printf '%s\n' "$CLEAN_CRON"
  printf '%s\n' "$CRON_BEGIN"
  printf '*/30 * * * * %s\n' "$RUNTIME_PULSE_TRIGGER"
  printf '* * * * * %s scheduled\n' "$RUNTIME_TRIGGER_SCRIPT"
  printf '45 2 * * 1 %s snapshot\n' "$RUNTIME_TRIGGER_SCRIPT"
  printf '%s\n' "$CRON_END"
} | crontab -

echo "local backend: gui/$UID_NUMBER/$LABEL"
echo "local news trigger: */30 * * * * $RUNTIME_PULSE_TRIGGER"
echo "local tool trigger: * * * * * $RUNTIME_TRIGGER_SCRIPT scheduled"
echo "local snapshot trigger: 45 2 * * 1 $RUNTIME_TRIGGER_SCRIPT snapshot"
echo "local derived refresh: pulse success -> gendaily -> hotpass"
echo "local database: $LOCAL_DB"
echo "server log: $LOG_DIR/server.log"
echo "discovertools log: $LOG_DIR/discovertools.log"
