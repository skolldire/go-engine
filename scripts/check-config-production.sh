#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
mode=${1:-all}
results=${CONFIG_RESULTS_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/config-validation.XXXXXX")}
mkdir -p "$results"
results=$(cd "$results" && pwd)
cd "$repo/config"
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
go version | tee "$results/environment.log"
go env -json GOOS GOARCH GOVERSION CGO_ENABLED >> "$results/environment.log"
run(){ local name=$1;shift; "$@" 2>&1 | tee "$results/$name.log"; }
quick(){
 run race-default go test -race -count=20 -timeout=120s ./...
 run race-1 env GOMAXPROCS=1 go test -race -count=20 -timeout=120s ./...
 run race-4 env GOMAXPROCS=4 go test -race -count=20 -timeout=120s ./...
 run vet go vet ./...
}
stress(){ run stress env CONFIG_STRESS=1 go test -race -run '^TestStress' -count=3 -v -timeout=180s ./...; }
fuzz(){
 for target in JSON References DecodeAndPaths Merge; do
  run "fuzz-$target" go test -run '^$' -fuzz "^Fuzz$target$" -fuzztime="${CONFIG_FUZZ_TIME:-30s}" -parallel=2
 done
}
bench(){ run benchmarks go test -run '^$' -bench . -benchmem -benchtime="${CONFIG_BENCH_TIME:-500ms}" -count=3 -timeout=180s; }
case "$mode" in quick) quick ;; stress) stress ;; fuzz) fuzz ;; bench) bench ;; all) quick;stress;fuzz;bench ;; *) echo 'expected quick|stress|fuzz|bench|all';exit 2 ;; esac
printf 'results=%s\n' "$results"
