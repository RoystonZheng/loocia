# --- CI / deploy targets (unchanged behavior) ---
# build.sh runs `make` (= all = install) with its own GOROOT (go1.24.4) and
# GOSUMDB=off, so these targets must NOT inherit the dev toolchain pin below.
all: install

install: clean gen_output
	mkdir -p output/bin
	go build -o output/bin/aihot-server

gen_output:
	mkdir -p output/log
	mkdir -p output/conf
	cp -rf conf/* output/conf/
	cp -rf control.sh output/

clean:
	rm -rf output/bin
	rm -rf output/conf
	rm -rf output/control.sh

# --- local dev targets ---
# The nuwa dep git.xiaojukeji.com/nuwa/golibs/json (sonic) panics at init() under
# go1.26, so on a go1.26-default machine plain `go run main.go` crashes. Pin the
# toolchain to go1.25.5 for THESE targets only (target-scoped export — does not
# leak into `install`/CI, and no machine-wide `go env -w`). GOSUMDB is set so the
# pinned toolchain can be verified/fetched on a machine with GOSUMDB=off.
# AIHOT_DATABASE_URL is overridable: `AIHOT_DATABASE_URL=... make run`.
AIHOT_DATABASE_URL ?= postgres://aihot:aihot@localhost:5432/aihot_test

run test build: export GOTOOLCHAIN := go1.25.5
run test build: export GOSUMDB := sum.golang.org
run: export AIHOT_DATABASE_URL := $(AIHOT_DATABASE_URL)

run:
	go run main.go
# -p 1: DB-backed test packages (items/ingest/daily/pipeline/publicapi) share the
# same live tables and TRUNCATE them — parallel packages corrupt each other.
test:
	go test -p 1 ./...
build:
	go build ./...

.PHONY: all install gen_output clean run test build
