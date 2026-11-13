#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
mode=${1:-all}
results=${RETRY_RESULTS_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/retry-validation.XXXXXX")}
mkdir -p "$results"
results=$(cd "$results" && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
cd "$repo/retry"
version=$(go env GOVERSION)
if [[ ! "$version" =~ ^go1\.27\.([0-9]+)$ ]] || (( ${BASH_REMATCH[1]:-0} < 4 ));then echo "requires Go 1.27.4 or a later 1.27 patch, got $version";exit 1;fi
go version | tee "$results/environment.log"
go env -json GOOS GOARCH GOVERSION CGO_ENABLED >> "$results/environment.log"
run(){ local name=$1;shift;"$@" 2>&1 | tee "$results/$name.log"; }
quick(){
 run race-default go test -race -count=20 -timeout=120s ./...
 run race-1 env GOMAXPROCS=1 go test -race -count=20 -timeout=120s ./...
 run race-4 env GOMAXPROCS=4 go test -race -count=20 -timeout=120s ./...
 run vet go vet ./...
}
stress(){ run stress env RETRY_STRESS=1 go test -race -run '^TestStress' -v -count=3 -timeout=180s ./...; }
fuzz(){ for target in Config Backoff Decisions;do run "fuzz-$target" go test -run '^$' -fuzz "^Fuzz$target$" -fuzztime="${RETRY_FUZZ_TIME:-30s}" -parallel=2;done; }
bench(){ run benchmarks go test -run '^$' -bench . -benchmem -benchtime="${RETRY_BENCH_TIME:-500ms}" -count=3 -timeout=180s; }
case "$mode" in quick)quick ;; stress)stress ;; fuzz)fuzz ;; bench)bench ;; all)quick;stress;fuzz;bench ;; *)echo 'expected quick|stress|fuzz|bench|all';exit 2 ;; esac
printf 'results=%s\n' "$results"
