#!/usr/bin/env bash
# Builds a throwaway module outside this repository that imports the published
# packages, with GOWORK=off.
#
# Scope, honestly: in working-tree mode this proves the import graph is sound —
# every package a consumer would import exists and compiles together. It does
# NOT prove the requires are resolvable, because replacing the core satisfies
# them regardless of the version they name. scripts/check-module-versions.sh is
# the gate for that; run it too.
#
# Tag mode is the real end-to-end check, and can only run once the tags exist.
#
# Usage:
#   scripts/smoke-external-consumer.sh              # against the working tree
#   scripts/smoke-external-consumer.sh v0.30.0      # against published tags
set -euo pipefail

VERSION="${1:-}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

cd "$WORK"
cat > main.go <<'GO'
// Imports one package from every module, so a broken require in any of them
// fails the build.
package main

import (
	_ "github.com/skolldire/go-engine/aws/provider/sqs"
	_ "github.com/skolldire/go-engine/database/memcached/provider/memcached"
	_ "github.com/skolldire/go-engine/database/mongodb/provider/mongodb"
	_ "github.com/skolldire/go-engine/database/redis/provider/redis"
	_ "github.com/skolldire/go-engine/http/provider/rest"
	_ "github.com/skolldire/go-engine/messaging/provider/kafka"
	_ "github.com/skolldire/go-engine/pkg/engine"
	_ "github.com/skolldire/go-engine/preset/full"
)

func main() {}
GO

# The consumer declares the same floor the modules promise. Pinning the
# toolchain to it is what makes that promise tested rather than assumed: without
# GOTOOLCHAIN, Go silently builds with whatever is installed locally (1.26.x
# here), so a dependency that had quietly started needing a newer Go would still
# pass — and break only for a consumer actually on the declared minimum.
GO_FLOOR=$(awk '/^go /{print $2; exit}' "$REPO/go.mod")

cat > go.mod <<GOMOD
module example.com/smoke

go $GO_FLOOR
GOMOD

MODULES="
github.com/skolldire/go-engine:$REPO
github.com/skolldire/go-engine/aws:$REPO/aws
github.com/skolldire/go-engine/messaging:$REPO/messaging
github.com/skolldire/go-engine/http:$REPO/http
github.com/skolldire/go-engine/database/sql:$REPO/database/sql
github.com/skolldire/go-engine/database/redis:$REPO/database/redis
github.com/skolldire/go-engine/database/mongodb:$REPO/database/mongodb
github.com/skolldire/go-engine/database/memcached:$REPO/database/memcached
github.com/skolldire/go-engine/preset/full:$REPO/preset/full
"

if [[ -z "$VERSION" ]]; then
    echo "==> smoke test against the working tree (import graph only)"
    for entry in $MODULES; do
        echo "replace ${entry%%:*} => ${entry##*:}" >> go.mod
    done
else
    echo "==> smoke test against published tags ($VERSION)"
    for entry in $MODULES; do
        echo "require ${entry%%:*} $VERSION" >> go.mod
    done
fi

export GOWORK=off
export GOTOOLCHAIN="go$GO_FLOOR"

echo "==> building with Go $GO_FLOOR (the floor declared in go.mod)"
go mod tidy
go build ./...

if [[ -z "$VERSION" ]]; then
    echo "==> OK: the import graph builds outside the repo (versions unchecked;"
    echo "        run scripts/check-module-versions.sh for those)"
else
    echo "==> OK: an external consumer can install and build every module at $VERSION"
fi
