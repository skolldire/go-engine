#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mode=${1:-all}
results=${CONCURRENCY_RESULTS_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/concurrency-validation.XXXXXX")}
mkdir -p "$results"
results=$(cd "$results" && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off
export GOFLAGS=-mod=readonly
cd "$root/concurrency"

version=$(go env GOVERSION)
if [[ ! "$version" =~ ^go1\.27\.([0-9]+)$ ]] || (( ${BASH_REMATCH[1]:-0} < 4 )); then
 printf 'requires Go 1.27.4 or a newer 1.27 patch; found %s\n' "$version" >&2
 exit 1
fi
go version | tee "$results/environment.log"
go env -json GOOS GOARCH GOVERSION CGO_ENABLED >> "$results/environment.log"
printf 'GOMAXPROCS=%s\n' "${GOMAXPROCS:-runtime default}" >> "$results/environment.log"
printf 'results=%s\n' "$results"

run() {
 local name=$1
 shift
 "$@" 2>&1 | tee "$results/$name.log"
}

quick() {
 run race-default go test -race -count=20 -timeout=120s ./...
 run race-1 env GOMAXPROCS=1 go test -race -count=20 -timeout=120s ./...
 run race-4 env GOMAXPROCS=4 go test -race -count=20 -timeout=120s ./...
 run vet go vet ./...
}

stress() {
 run stress-default env CONCURRENCY_STRESS=1 go test -race -run '^TestStressPoolLifecycle$' -count=5 -timeout=180s ./...
 run stress-1 env GOMAXPROCS=1 CONCURRENCY_STRESS=1 go test -race -run '^TestStressPoolLifecycle$' -count=5 -timeout=180s ./...
 run stress-4 env GOMAXPROCS=4 CONCURRENCY_STRESS=1 go test -race -run '^TestStressPoolLifecycle$' -count=5 -timeout=180s ./...
 run sustained env CONCURRENCY_STRESS=1 go test -race -run '^TestStressSustainedWorkload$' -count=1 -timeout=90s ./...
 run retention env CONCURRENCY_STRESS=1 go test -race -run '^TestStressResourcePlateau$' -v -count=3 -timeout=180s ./...
}

fuzz() {
 run fuzz-config go test -run '^$' -fuzz '^FuzzPoolConfig$' -fuzztime="${CONCURRENCY_FUZZ_TIME:-60s}" -parallel=2
 run fuzz-transitions go test -run '^$' -fuzz '^FuzzPoolTransitions$' -fuzztime="${CONCURRENCY_FUZZ_TIME:-60s}" -parallel=2
}

bench() {
 run benchmarks go test -run '^$' -bench . -benchmem -benchtime="${CONCURRENCY_BENCH_TIME:-1s}" -count=3 -timeout=180s
}

case "$mode" in
 quick) quick ;;
 stress) stress ;;
 fuzz) fuzz ;;
 bench) bench ;;
 all) quick; stress; fuzz; bench ;;
 *) printf 'usage: %s [quick|stress|fuzz|bench|all]\n' "$0" >&2; exit 2 ;;
esac
