#!/usr/bin/env bash
# Both consumers run offline against the candidate, without the engine/workspace.
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
export GOWORK=off
export GOPROXY=off
export GOTOOLCHAIN=local
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-mod=readonly"
MODULE=github.com/skolldire/go-engine/concurrency
"$REPO/scripts/check-concurrency-deps.sh"
for name in minimal composed; do
    cd "$REPO/examples/standalone/concurrency/$name"
    echo "==> standalone concurrency/$name (offline, GOWORK=off)"
    allowed="example.com/concurrency-$name"
    module_paths=$(go list -m -f '{{.Path}}' all)
    while IFS= read -r mod; do
        [[ "$mod" == "$MODULE" || "$mod" == "$allowed" ]] || {
            echo "FAIL: unexpected module $mod"; exit 1;
        }
    done <<< "$module_paths"
    package_modules=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
    while IFS= read -r mod; do
        [[ -z "$mod" || "$mod" == "$MODULE" || "$mod" == "$allowed" ]] || {
            echo "FAIL: unexpected runtime/test dependency $mod"; exit 1;
        }
    done <<< "$package_modules"
    # Include graph edges, even requirements unused by the selected packages.
    graph=$(go mod graph)
    while read -r from to; do
        for node in "$from" "$to"; do
            case "$node" in
                "$allowed"|"$MODULE"@*|go@*|toolchain@*) ;;
                *) echo "FAIL: unexpected graph node $node"; exit 1 ;;
            esac
        done
    done <<< "$graph"
    replaces=$(go list -m -f '{{with .Replace}}{{.Path}}{{end}}' all | sed '/^$/d')
    [[ "$replaces" == '../../../../concurrency' ]] || {
        echo "FAIL: only the documented concurrency replacement is permitted"; exit 1;
    }
    go run .
    go test -race ./...
done
echo '==> OK: minimal and composed, only stdlib + concurrency'
