#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
cd "$repo/client"
module=github.com/skolldire/go-engine/client
modules=$(go list -m -f '{{.Path}}' all)
[[ "$modules" == "$module" ]] || { echo 'client must have zero requirements';exit 1; }
packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
while IFS= read -r path;do
 [[ -z "$path" || "$path" == "$module" ]] || { echo "forbidden client dependency: $path";exit 1; }
done <<< "$packages"
for owner in concurrency config config/viper utilities;do
 [[ -f "$repo/$owner/go.mod" ]] || continue
 cd "$repo/$owner"
 dependencies=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 while IFS= read -r dependency;do
  case "$dependency" in "$module"|"$module"/*) echo "$owner must not import client (D26)";exit 1 ;; esac
 done <<< "$dependencies"
done
echo 'client: stdlib-only runtime/tests; D26 inverse dependency boundaries pass'
