#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/../.." && pwd)
case "$1" in
 deps) ;;
 engine)cd "$repo/harness/config-engine-comparison";GOWORK=off go test -race -count=1 ./... ;;
 bench-engine)cd "$repo/harness/config-engine-comparison";GOWORK=off go test -run '^$' -bench '^BenchmarkConfigDecoderComparison$' -benchmem -count=3 -benchtime=500ms ./... ;;
 *)echo 'unknown config hook' >&2;exit 2 ;;
esac
