#!/usr/bin/env bash
# Daily pg_dump of the aihot production DB, keep 7 days.
set -euo pipefail
DIR=/root/aihot/backups
mkdir -p "$DIR"
pg_dump "postgres://aihot:aihot@localhost:5432/aihot" -Fc -f "$DIR/aihot-$(date +%F).dump"
find "$DIR" -name "aihot-*.dump" -mtime +7 -delete
