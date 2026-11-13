#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
cd "$repo/retry"
module=github.com/skolldire/go-engine/retry
[[ "$(go list -m -f '{{.Path}}' all)" == "$module" ]] || { echo 'retry must have zero requirements';exit 1; }
packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
while IFS= read -r path;do
 [[ -z "$path" || "$path" == "$module" ]] || { echo "forbidden retry dependency: $path";exit 1; }
done <<< "$packages"
echo 'retry: stdlib-only runtime and tests'
