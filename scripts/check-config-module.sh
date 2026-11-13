#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
"$repo/scripts/check-config-deps.sh"
for name in minimal composed; do
 cd "$repo/examples/standalone/config/$name"
 consumer="example.com/config-$name"
 modules=$(go list -m -f '{{.Path}}' all)
 packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 while IFS= read -r path; do
  case "$path" in ''|"$consumer"|github.com/skolldire/go-engine/config) ;; github.com/skolldire/go-engine/concurrency) [[ "$name" == composed ]] || exit 1 ;; *) echo "forbidden dependency $path"; exit 1 ;; esac
 done <<< "$modules
$packages"
 graph=$(go mod graph)
 while read -r from to; do
  for node in "$from" "$to"; do
   case "$node" in "$consumer"|github.com/skolldire/go-engine/config@v0.0.0|go@*|toolchain@*) ;; github.com/skolldire/go-engine/concurrency@v0.0.0) [[ "$name" == composed ]] || exit 1 ;; *) echo "forbidden graph node $node";exit 1 ;; esac
  done
 done <<< "$graph"
 replacements=$(go list -m -f '{{with .Replace}}{{.Path}}{{end}}' all | sed '/^$/d' | sort)
 expected=../../../../config
 if [[ "$name" == composed ]]; then expected=$(printf '%s\n' ../../../../config ../../../../concurrency | sort); fi
 [[ "$replacements" == "$expected" ]] || { echo 'unexpected replacements';exit 1; }
 go run .
 go test -race ./...
done
