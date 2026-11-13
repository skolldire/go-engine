#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
version=${1:-v1.0.0}
mode=${2:-published}
module=github.com/skolldire/go-engine/concurrency
[[ "$version" =~ ^v1\.[0-9]+\.[0-9]+$ ]] || { echo 'expected a stable v1 semantic version' >&2; exit 2; }
[[ "$mode" == published || "$mode" == --fixture ]] || { echo 'unknown mode' >&2; exit 2; }
if [[ "$mode" == published ]]; then
 case "${GOPROXY:-https://proxy.golang.org,direct}" in
  *file:*|off) echo 'published validation requires a remote module source' >&2; exit 2 ;;
 esac
fi
results=${CONCURRENCY_RELEASE_RESULTS_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/concurrency-release-results.XXXXXX")}
mkdir -p "$results"
results=$(cd "$results" && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/concurrency-release-check.XXXXXX")
cleanup() {
 chmod -R u+w "$work"
 rm -rf "$work"
}
trap cleanup EXIT
export GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly
export GOMODCACHE="$work/modcache" GOPATH="$work/gopath"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
go version > "$results/environment.log"
printf 'mode=%s\nmodule=%s\nversion=%s\nproxy=%s\n' "$mode" "$module" "$version" "$GOPROXY" >> "$results/environment.log"

check_dependencies() {
 local consumer=$1
 local modules packages graph replacements selected node
 selected=$(go list -m -f '{{.Version}}' "$module")
 [[ "$selected" == "$version" ]] || { echo "wrong version: $selected" >&2; return 1; }
 replacements=$(go list -m -f '{{with .Replace}}{{.Path}}{{end}}' all | sed '/^$/d')
 [[ -z "$replacements" ]] || { echo 'replacements are forbidden' >&2; return 1; }
 modules=$(go list -m -f '{{.Path}}' all)
 packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 while IFS= read -r node; do
  [[ -z "$node" || "$node" == "$consumer" || "$node" == "$module" ]] || { echo "unexpected module: $node" >&2; return 1; }
 done <<< "$modules
$packages"
 graph=$(go mod graph)
 while read -r from to; do
  for node in "$from" "$to"; do
   case "$node" in "$consumer"|"$module@$version"|go@*|toolchain@*) ;; *) echo "unexpected graph node: $node" >&2; return 1 ;; esac
  done
 done <<< "$graph"
 go list -m -json all
 printf '%s\n' "$graph"
}

for name in minimal composed; do
 mkdir "$work/$name"
 cd "$work/$name"
 consumer="example.com/concurrency-release-$name"
 cp "$repo/examples/standalone/concurrency/$name/main.go" .
 go mod init "$consumer" > "$results/$name.log" 2>&1
 go get "$module@$version" >> "$results/$name.log" 2>&1
 go mod download -json "$module@$version" > "$results/$name-download.json"
 downloaded=$(go list -m -f '{{.Dir}}' "$module")
 sed 's|  concurrency/|  |' "$repo/docs/modules/concurrency-candidate.sha256" > "$work/expected.sha256"
 (
  cd "$downloaded"
  if command -v sha256sum >/dev/null; then sha256sum -c "$work/expected.sha256"; else shasum -a 256 -c "$work/expected.sha256"; fi
 ) >> "$results/$name.log" 2>&1
 check_dependencies "$consumer" >> "$results/$name.log" 2>&1
 go run . >> "$results/$name.log" 2>&1
 go test -race ./... >> "$results/$name.log" 2>&1
 GOPROXY=off go run . >> "$results/$name.log" 2>&1
 GOPROXY=off go test -race ./... >> "$results/$name.log" 2>&1
 cp go.mod "$results/$name.go.mod"
 cp go.sum "$results/$name.go.sum"
 echo "$name: downloaded $version, source hashes matched, online/offline consumers passed"
done
cleanup
trap - EXIT
if [[ "$mode" == --fixture ]]; then
 echo 'FIXTURE ONLY: published-tag evidence E7 remains pending'
else
 echo "E7 consumers passed for $module@$version; retain logs with approved commit/tag evidence"
fi
printf 'results=%s\n' "$results"
