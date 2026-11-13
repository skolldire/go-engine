#!/usr/bin/env bash
# Architecture boundary for both runtime and test dependencies of concurrency.
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
export GOWORK=off GOPROXY=off GOTOOLCHAIN=local
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-mod=readonly"
cd "$REPO/concurrency"
MODULE=github.com/skolldire/go-engine/concurrency
modules=$(go list -m all)
[[ "$modules" == "$MODULE" ]] || {
    echo 'FAIL: concurrency go.mod must have no dependencies (including test/tool deps)'
    echo "$modules"
    exit 1
}
packages=$(go list -deps -test -f '{{if not .Standard}}{{with .Module}}{{.Path}}{{end}}{{end}}' ./...)
while IFS= read -r mod; do
    [[ -z "$mod" || "$mod" == "$MODULE" ]] || {
        echo "FAIL: concurrency resolved non-stdlib dependency $mod"; exit 1;
    }
done <<< "$packages"
echo '==> OK: concurrency runtime and tests are stdlib-only (GOWORK=off)'
