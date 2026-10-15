#!/usr/bin/env bash
# Restart the LaunchAgent after source changes. The cron trigger reads the
# current source tree on its next tick.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$SCRIPT_DIR/install-local-aihot.sh"
