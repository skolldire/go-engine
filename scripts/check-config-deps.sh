#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo/config"
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
module=github.com/skolldire/go-engine/config
modules=$(go list -m -f '{{.Path}}' all)
[[ "$modules" == "$module" ]] || { echo 'config must not require other modules'; exit 1; }
packages=$(go list -deps -test -f '{{if not .Standard}}{{with .Module}}{{.Path}}{{end}}{{end}}' ./...)
while IFS= read -r path; do
 [[ -z "$path" || "$path" == "$module" ]] || { echo "forbidden config dependency: $path"; exit 1; }
done <<< "$packages"
echo 'config dependencies: only stdlib, including tests'
