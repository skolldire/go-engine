#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
cd "$repo/ratelimit"
module=github.com/skolldire/go-engine/ratelimit
[[ "$(go list -m -f '{{.Path}}' all)" == "$module" ]] || { echo 'ratelimit must have zero requirements';exit 1; }
packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
while IFS= read -r path;do
 [[ -z "$path" || "$path" == "$module" ]] || { echo "forbidden ratelimit dependency: $path";exit 1; }
done <<< "$packages"
echo 'ratelimit: stdlib-only runtime and tests'
