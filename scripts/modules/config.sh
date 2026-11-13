#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/../.." && pwd)
case "$1" in
 deps) ;;
 engine)cd "$repo";GOWORK="$repo/go.work" go test -tags=configcomparison -race -count=1 ./pkg/engine ;;
 bench-engine)cd "$repo";GOWORK="$repo/go.work" go test -tags=configcomparison -run '^$' -bench '^BenchmarkConfigDecoderComparison$' -benchmem -count=3 -benchtime=500ms ./pkg/engine ;;
 *)echo 'unknown config hook' >&2;exit 2 ;;
esac
