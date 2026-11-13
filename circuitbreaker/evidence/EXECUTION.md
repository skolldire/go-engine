# Execution record

Initial HEAD d87e888 (retry), feature/split, porcelain empty. Go1.27.4 darwin/arm64 (Apple M1 Max) and linux/arm64 (local Colima container). No AMD64 execution. Root regression/lint/gorelease ran on macOS; Linux ran module qualification and both independent consumers, not the full legacy suite.

## Native commands

```sh
export PATH=/private/tmp/go-engine-toolchain/go/bin:/private/tmp/circuitbreaker-tools/bin:$PATH
export GOCACHE=/private/tmp/go-engine-build-cache GOPROXY=off
export GOMODCACHE=/private/tmp/go-engine-mod-cache
export CIRCUITBREAKER_RESULTS_DIR="$PWD/circuitbreaker/evidence/darwin-arm64"
./scripts/check-circuitbreaker-production.sh all
export GOMODCACHE=/Users/seven/go/pkg/mod
export GOLANGCI_LINT_CACHE=/private/tmp/go-engine-lint-cache
make lint
make lint-arch lint-docs lint-deps check-circuitbreaker check-retry check-client check-config check-concurrency
make test
go test -race -count=1 ./pkg/utilities/circuit_breaker/... ./pkg/utilities/resilience/...
go test -run '^$' -bench '^BenchmarkBoundarySuccess$' -benchmem -benchtime=500ms -count=3 ./pkg/utilities/circuit_breaker
(cd circuitbreaker && GOWORK=off go test -run '^$' -bench '^BenchmarkClosed/success$' -benchmem -benchtime=500ms -count=3)
```

Qualification script sets GOWORK=off/GOTOOLCHAIN=local/GOPROXY=off/GOFLAGS=-mod=readonly. Quick: go test -race -count=20 -timeout=120s ./... at default GOMAXPROCS, 1 and 4, then vet. Stress: CIRCUITBREAKER_STRESS=1, race, count=3. Config/Rate/Transitions/Rollover fuzz campaigns: 30s each, two workers. Benchmarks: 500ms/count=3; RunParallel uses PB.Next. CIRCUITBREAKER_RESULTS_DIR defaults to a new temporary directory; campaigns do not silently overwrite repository evidence unless explicitly configured.

The linter symlink pointed to /private/tmp/golangci-lint-2.14.0-darwin-arm64/golangci-lint. Existing repository dependencies were read from the user's cache. Sandbox guards emitted harmless cache-stat write warnings but passed. Full repository tests required permission for existing local HTTP/gRPC listeners; the new module and consumers open no sockets. No dependency versions changed.

`make check-modules` was additionally executed and failed with the existing common-version policy: v0.0.0 versus v0.30.0 and last root tag v0.22.0. version-baseline.log records HEAD's existing config/viper requirement. Exact output in darwin-arm64/check-modules-deferred.log. This is E6 distribution work, not a passed check; make all is not claimed to pass. No manifest/release workflow was changed.

## Linux ARM64

Image sha256:e432b43af23a9328d56a7c499be0476810aa344acbcf65fc7c455d4ff5a40602. Flags: --network none --read-only --cpus 4 --memory 4g --tmpfs /tmp:rw,exec,size=3g. Repository mounted read-only at /src; evidence/linux-arm64 mounted writable at /results. PATH=/tmp/go/bin:$PATH, GOCACHE=/tmp/cache, GOMODCACHE=/tmp/modcache, CIRCUITBREAKER_RESULTS_DIR=/results.

The existing /private/tmp/config-linux-validation.tar.gz supplied Go1.27.4 and cache through stdin (`tar -xz -C /tmp`). Official Linux ARM64 toolchain archive SHA256: 3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92. New module/consumers need only stdlib and explicit local sibling modules.

```sh
/src/scripts/check-circuitbreaker-production.sh all
/src/scripts/check-circuitbreaker-module.sh > /results/consumers.log 2>&1
```

## Compatibility and diagnostic commands

Copied go.mod and *.go to a fresh temporary directory outside Git; ran externally installed /private/tmp/go-engine-tools/gorelease -base=none -version=v0.1.0, exit 0. First module version, no historical API comparison or fabricated tags/proxy. Tool dependency is outside the module.

Diagnostic coverage: from circuitbreaker, `GOWORK=off go test -coverprofile=evidence/darwin-arm64/coverage.out ./...` followed by `go tool cover -func=...`. Coverage is not a proof of correctness; explicit contracts, independent model and rational arithmetic oracle provide behavioral evidence.

## Hash verification

From repository root: `shasum -a 256 -c circuitbreaker/evidence/integration-sources.sha256`.
From circuitbreaker: `shasum -a 256 -c SOURCE.sha256`.
From circuitbreaker/evidence: `shasum -a 256 -c SHA256SUMS`.

Source manifest covers module code/tests/docs. Integration manifest covers root integration edits, consumers, scripts and test-only legacy benchmark. Evidence manifest excludes itself. No staged changes, commit, push or tags.
