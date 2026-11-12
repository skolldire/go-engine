#!/usr/bin/env bash
# Prove the leaf module has no hidden workspace or engine dependency.
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
export GOWORK=off
export GOPROXY=off
export GOTOOLCHAIN=local
cd "$REPO/concurrency"
modules=$(go list -m all)
if [[ "$modules" != 'github.com/skolldire/go-engine/concurrency' ]]; then
    echo "FAIL: concurrency must have no module dependencies"
    echo "$modules"
    exit 1
fi
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
cd "$WORK"
go mod init concurrency-consumer >/dev/null
go mod edit -require=github.com/skolldire/go-engine/concurrency@v0.0.0
go mod edit "-replace=github.com/skolldire/go-engine/concurrency=$REPO/concurrency"
cat > main.go <<'GO'
package main

import (
    "context"
    "github.com/skolldire/go-engine/concurrency"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    output, err := concurrency.Map(ctx, concurrency.From(ctx, 21), concurrency.Config{Workers: 1},
        func(_ context.Context, n int) (int, error) { return n * 2, nil })
    if err != nil { panic(err) }
    result := <-output
    if result.Err != nil || result.Value != 42 { panic("unexpected result") }
}
GO
go mod tidy
go run .
echo '==> OK: standalone concurrency consumer, zero external dependencies'
