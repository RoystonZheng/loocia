#!/usr/bin/env bash
# Cross-compile the pipeline binaries for Melos (linux/amd64, static).
set -euo pipefail
cd "$(dirname "$0")/../server"
OUT="../deploy/out"
mkdir -p "$OUT"
export GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64
for cmd in pulse gendaily hotpass; do
  echo "building $cmd..."
  go build -o "$OUT/$cmd" "./cmd/$cmd"
done
echo "built: $(ls -1 "$OUT")"
