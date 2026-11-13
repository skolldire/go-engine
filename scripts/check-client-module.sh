#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
"$repo/scripts/check-client-deps.sh"
module=github.com/skolldire/go-engine/client
for name in minimal composed;do
 cd "$repo/examples/standalone/client/$name"
 consumer="example.com/client-$name"
 modules=$(go list -m -f '{{.Path}}' all)
 packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 while IFS= read -r dependency;do
  case "$dependency" in ''|"$consumer"|"$module") ;; *) echo "unexpected consumer dependency: $dependency";exit 1 ;; esac
 done <<< "$modules
$packages"
 graph=$(go mod graph)
 while read -r from to;do
  for node in "$from" "$to";do case "$node" in "$consumer"|"$module"@v0.0.0|go@*|toolchain@*) ;; *) echo "unexpected graph node: $node";exit 1 ;; esac;done
 done <<< "$graph"
 replacements=$(go list -m -f '{{with .Replace}}{{.Path}}{{end}}' all | sed '/^$/d')
 [[ "$replacements" == ../../../../client ]] || { echo 'unexpected replace';exit 1; }
 go run .
 go test -race ./...
done
