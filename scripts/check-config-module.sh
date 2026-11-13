#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
"$repo/scripts/check-config-deps.sh"
for name in minimal composed; do
 cd "$repo/examples/standalone/config/$name"
 consumer="example.com/config-$name"
 allowlist=config
 [[ "$name" == composed ]] && allowlist=config-viper
 allowed(){
  case "$1" in ''|"$consumer"|github.com/skolldire/go-engine/config|go|toolchain) return 0 ;;
   github.com/skolldire/go-engine/concurrency|github.com/skolldire/go-engine/config/viper) [[ "$name" == composed ]];return ;;
  esac
  awk -v module="$1" '$1==module {found=1} END {exit !found}' "$repo/scripts/allowlists/$allowlist-modules.txt"
 }
 modules=$(go list -m -f '{{.Path}}' all)
 packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 while IFS= read -r module; do allowed "$module" || { echo "forbidden module: $module";exit 1; };done <<< "$modules
$packages"
 selected=$(go list -m -f '{{if .Version}}{{.Path}} {{.Version}}{{end}}' all | sed '/^$/d' | sed '\|^github.com/skolldire/|d' | sort)
 versions=$allowlist
 [[ "$name" == composed ]] && versions=config-composed
 expected=$(cat "$repo/scripts/allowlists/$versions-modules.txt")
 [[ "$selected" == "$expected" ]] || { echo "consumer versions differ: $name";diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$selected") || true;exit 1; }
 graph=$(go mod graph)
 while read -r from to; do
  for node in "$from" "$to"; do allowed "${node%@*}" || { echo "forbidden graph node: $node";exit 1; };done
 done <<< "$graph"
 replacements=$(go list -m -f '{{with .Replace}}{{.Path}}{{end}}' all | sed '/^$/d' | sort)
 expected=../../../../config
 [[ "$name" == composed ]] && expected=$(printf '%s\n' ../../../../config ../../../../config/viper ../../../../concurrency | sort)
 [[ "$replacements" == "$expected" ]] || { echo 'unexpected replacements';exit 1; }
 go run .
 go test -race ./...
done
