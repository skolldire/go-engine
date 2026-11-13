#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
cd "$repo/resilience"
modules=$(go list -m -f '{{.Path}}' all)
expected=$(printf '%s\n' circuitbreaker client concurrency ratelimit resilience retry | sed 's|^|github.com/skolldire/go-engine/|')
[[ "$(printf '%s\n' "$modules" | sort)" == "$expected" ]] || { echo 'unexpected resilience module graph';exit 1; }
packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
while IFS= read -r path;do
 case "$path" in ''|github.com/skolldire/go-engine/resilience|github.com/skolldire/go-engine/client|github.com/skolldire/go-engine/retry|github.com/skolldire/go-engine/ratelimit|github.com/skolldire/go-engine/circuitbreaker|github.com/skolldire/go-engine/concurrency) ;; *)echo "forbidden resilience dependency: $path";exit 1 ;; esac
done <<< "$packages"
for owner in client retry ratelimit concurrency circuitbreaker;do
 cd "$repo/$owner"
 dependencies=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 while IFS= read -r dependency;do
  [[ "$dependency" != github.com/skolldire/go-engine/resilience ]] || { echo "$owner must not import resilience";exit 1; }
 done <<< "$dependencies"
done
echo 'resilience: stdlib + five base modules only, no inverse dependency'
