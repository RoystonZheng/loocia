#!/usr/bin/env bash
# Cross-compile the pipeline + server binaries for Melos (linux/amd64, static),
# and build the web bundle. Outputs to deploy/out/.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/deploy/out"
mkdir -p "$OUT"

echo "== go binaries (linux/amd64 static) =="
cd "$ROOT/server"
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.25.5}" GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64
for cmd in pulse gendaily hotpass discovertools; do
  echo "  building $cmd..."
  go build -o "$OUT/$cmd" "./cmd/$cmd"
done
echo "  building server..."
go build -o "$OUT/server" .

echo "== web bundle =="
cd "$ROOT/web"
npm run build >/dev/null
rm -rf "$OUT/web"
cp -r dist "$OUT/web"

echo "built: $(ls -1 "$OUT" | tr '\n' ' ')"
