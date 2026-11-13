#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/../.." && pwd)
case "$1" in
 deps)return_code=0 ;;
 stress)
 cd "$repo/$2"
 run(){ local label=$1;shift;"$@" 2>&1 | tee "$MODULE_RESULTS_DIR/$label.log"; }
 run stress-default env CONCURRENCYSTRESS=1 CONCURRENCY_STRESS=1 go test -race -run '^TestStressPoolLifecycle$' -count=5 -timeout=180s ./...
 run stress-1 env GOMAXPROCS=1 CONCURRENCYSTRESS=1 CONCURRENCY_STRESS=1 go test -race -run '^TestStressPoolLifecycle$' -count=5 -timeout=180s ./...
 run stress-4 env GOMAXPROCS=4 CONCURRENCYSTRESS=1 CONCURRENCY_STRESS=1 go test -race -run '^TestStressPoolLifecycle$' -count=5 -timeout=180s ./...
 run sustained env CONCURRENCY_STRESS=1 go test -race -run '^TestStressSustainedWorkload$' -count=1 -timeout=90s ./...
 run retention env CONCURRENCY_STRESS=1 go test -race -run '^TestStressResourcePlateau$' -v -count=3 -timeout=180s ./... ;;
 *)echo 'unknown concurrency hook' >&2;exit 2 ;;
esac
