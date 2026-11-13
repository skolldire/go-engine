#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
"$repo/scripts/check-circuitbreaker-deps.sh"
for name in minimal composed;do
 cd "$repo/examples/standalone/circuitbreaker/$name"
 consumer="example.com/circuitbreaker-$name"
 modules=$(go list -m -f '{{.Path}}' all)
 packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 while IFS= read -r dependency;do
  case "$dependency" in ''|"$consumer"|github.com/skolldire/go-engine/circuitbreaker) ;;
   github.com/skolldire/go-engine/client|github.com/skolldire/go-engine/retry) [[ "$name" == composed ]] || exit 1 ;;
   *) echo "unexpected consumer dependency: $dependency";exit 1 ;; esac
 done <<< "$modules
$packages"
 graph=$(go mod graph)
 while read -r from to;do
  for node in "$from" "$to";do case "$node" in "$consumer"|github.com/skolldire/go-engine/circuitbreaker@v0.0.0|go@*|toolchain@*) ;;
   github.com/skolldire/go-engine/client@v0.0.0|github.com/skolldire/go-engine/retry@v0.0.0) [[ "$name" == composed ]] || exit 1 ;;
   *) echo "unexpected graph node: $node";exit 1 ;; esac;done
 done <<< "$graph"
 replacements=$(go list -m -f '{{with .Replace}}{{.Path}}{{end}}' all | sed '/^$/d' | sort)
 expected=../../../../circuitbreaker
 [[ "$name" != composed ]] || expected=$'../../../../circuitbreaker\n../../../../client\n../../../../retry'
 [[ "$replacements" == "$expected" ]] || { echo 'unexpected replace';exit 1; }
 go run .
 go test -race ./...
done
