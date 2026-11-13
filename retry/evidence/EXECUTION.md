# Execution record

Candidate base: e72fba5 (`add client module`), feature/split. Initial porcelain output was empty. Go 1.27.4, darwin/arm64 (Apple M1 Max) and linux/arm64 (local Colima container). No AMD64 run. Root regression/lint and gorelease ran on macOS; Linux ran the retry qualification and both standalone consumers, not the full legacy repository suite.

## Native commands

```sh
export PATH=/private/tmp/go-engine-toolchain/go/bin:/private/tmp/retry-tools/bin:$PATH
export GOCACHE=/private/tmp/go-engine-build-cache GOPROXY=off
export RETRY_RESULTS_DIR="$PWD/retry/evidence/darwin-arm64"
# New module/qualification, cache is writable:
export GOMODCACHE=/private/tmp/go-engine-mod-cache
./scripts/check-retry-production.sh quick
./scripts/check-retry-production.sh stress
./scripts/check-retry-production.sh fuzz
./scripts/check-retry-production.sh bench
# Existing repository dependencies were already available in this cache:
export GOMODCACHE=/Users/seven/go/pkg/mod
make lint
make lint-arch lint-docs lint-deps check-retry check-client check-config check-concurrency
make test
go test -race -count=1 ./pkg/utilities/retry_backoff/... ./pkg/utilities/resilience/...
go test -run '^$' -bench '^BenchmarkBoundarySuccess$' -benchmem -benchtime=500ms -count=3 ./pkg/utilities/retry_backoff
(cd retry && GOWORK=off go test -run '^$' -bench '^BenchmarkDoSuccess$' -benchmem -benchtime=500ms -count=3)
```

Qualification scripts use GOWORK=off, GOTOOLCHAIN=local, GOPROXY=off and GOFLAGS=-mod=readonly. quick runs race count=20/default, 1 and 4, then vet. stress uses RETRY_STRESS=1, race, count=3. Each fuzz target runs 30s/two workers. Benchmarks use 500ms/count=3; RunParallel uses PB.Next rather than b.Loop as required by that API.

Full repository tests needed sandbox access for existing local HTTP/gRPC listeners; retry itself opens none. Guard commands succeeded despite harmless legacy cache-stat write warnings under the sandbox. No dependency versions were changed.

Additional `make check-modules` failed: its existing shared-version policy sees v0.0.0 and v0.30.0, and v0.22.0 already tagged. HEAD:config/viper/go.mod already has config v0.0.0. This release-policy discrepancy is E6, outside this iteration; exact output is check-modules-deferred.log. It is not a passing check and make all is not claimed to pass. Release workflow/manifest were not modified.

## Linux

Container image e432b43af23a9328d56a7c499be0476810aa344acbcf65fc7c455d4ff5a40602, --network none, --read-only, 4 CPUs, 4GiB memory, /tmp tmpfs rw/exec/3GiB. Source mounted read-only at /src, only evidence/linux-arm64 writable at /results. The existing /private/tmp/config-linux-validation.tar.gz supplied Go 1.27.4 and cached modules through stdin; qualification and consumers require only stdlib and local modules. Linux toolchain archive SHA256: 3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92.

Container commands after `tar -xz -C /tmp`:
```sh
export PATH=/tmp/go/bin:$PATH GOCACHE=/tmp/cache GOMODCACHE=/tmp/modcache
export RETRY_RESULTS_DIR=/results
/src/scripts/check-retry-production.sh all
/src/scripts/check-retry-module.sh > /results/consumers.log 2>&1
```

## Compatibility

Copied go.mod and *.go into a temporary directory outside Git; ran externally installed `/private/tmp/go-engine-tools/gorelease -base=none -version=v0.1.0`, exit 0. First version, no historical base comparison, no fake tag or proxy. Tool installation is outside retry/go.mod.

## Evidence boundaries

Only final successful qualification runs are in platform logs; defects and fixture corrections are recorded in RELEASE_NOTES.md. Coverage is diagnostic only. Performance is fixture/host-specific, not an SLA. SOURCE.sha256 covers module code/tests/docs. integration-sources.sha256 covers consumers, guards and the legacy comparison test. SHA256SUMS covers evidence artifacts and excludes itself. Hashes are captured after final edits, with no staging, commits, pushes or tags.
