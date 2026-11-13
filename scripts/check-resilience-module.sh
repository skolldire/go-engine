#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
"$repo/scripts/check-resilience-deps.sh"
for name in minimal composed;do
 cd "$repo/examples/standalone/resilience/$name"
 consumer="example.com/resilience-$name"
 modules=$(go list -m -f '{{.Path}}' all)
 packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 while IFS= read -r dependency;do
  case "$dependency" in ''|"$consumer"|github.com/skolldire/go-engine/resilience|github.com/skolldire/go-engine/client|github.com/skolldire/go-engine/retry|github.com/skolldire/go-engine/concurrency|github.com/skolldire/go-engine/ratelimit|github.com/skolldire/go-engine/circuitbreaker) ;; *)echo "unexpected consumer dependency: $dependency";exit 1 ;; esac
 done <<< "$modules
$packages"
 [[ "$(printf '%s\n' "$modules" | wc -l | tr -d ' ')" == 7 ]] || { echo 'expected consumer plus six library modules';exit 1; }
 graph=$(go mod graph)
 while read -r from to;do
  for node in "$from" "$to";do case "$node" in "$consumer"|go@*|toolchain@*|github.com/skolldire/go-engine/resilience@v0.0.0|github.com/skolldire/go-engine/client@v0.0.0|github.com/skolldire/go-engine/retry@v0.0.0|github.com/skolldire/go-engine/concurrency@v0.0.0|github.com/skolldire/go-engine/ratelimit@v0.0.0|github.com/skolldire/go-engine/circuitbreaker@v0.0.0) ;; *) echo "unexpected graph node: $node";exit 1 ;; esac;done
 done <<< "$graph"
 replacements=$(go list -m -f '{{with .Replace}}{{.Path}}{{end}}' all | sed '/^$/d' | sort)
 expected=$(printf '%s\n' circuitbreaker client concurrency ratelimit resilience retry | sed 's|^|../../../../|')
 [[ "$replacements" == "$expected" ]] || { echo 'unexpected replace';exit 1; }
 go run .
 go test -race ./...
done
