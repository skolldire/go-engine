# Execution record

Initial HEAD 1d2760f (circuitbreaker), feature/split, porcelain empty. Go1.27.4 darwin/arm64 (Apple M1 Max) and linux/arm64 (local Colima container). No AMD64 execution. Root regression/lint/gorelease ran on macOS. Linux ran the new module's qualification and both independent consumers, not the full repository suite.

## Native commands

```sh
export PATH=/private/tmp/go-engine-toolchain/go/bin:/private/tmp/ratelimit-tools/bin:$PATH
export GOCACHE=/private/tmp/go-engine-build-cache GOPROXY=off
export GOMODCACHE=/private/tmp/go-engine-mod-cache
export RATELIMIT_RESULTS_DIR="$PWD/ratelimit/evidence/darwin-arm64"
./scripts/check-ratelimit-production.sh all
export GOMODCACHE=/Users/seven/go/pkg/mod
export GOLANGCI_LINT_CACHE=/private/tmp/go-engine-lint-cache
make lint
make lint-arch lint-docs lint-deps check-ratelimit check-circuitbreaker check-retry check-client check-config check-concurrency
make test
make check-modules
```

The final qualification/regression/lint/consumer runs followed reuse of integer temporary buffers; final platform logs supersede the initial qualification. No dependency versions changed. The linter symlink pointed to /private/tmp/golangci-lint-2.14.0-darwin-arm64/golangci-lint. Existing dependencies were already available in the user's cache. Full repository tests needed permission for legacy local HTTP/gRPC listeners; ratelimit and its consumers open no sockets. Legacy cache-stat warnings under sandbox guards were nonfatal.

Production script sets GOWORK=off/GOTOOLCHAIN=local/GOPROXY=off/GOFLAGS=-mod=readonly. Quick: race count=20 at default GOMAXPROCS, 1 and 4, timeout=120s, plus vet. Stress: RATELIMIT_STRESS=1, race/count=3/timeout=180s. Config/Credit/Accounting/Queue/Envelope fuzz campaigns: 30s each, two workers. Benchmarks: 500ms/count=3, without race; RunParallel uses PB.Next. RATELIMIT_RESULTS_DIR defaults to a fresh temporary directory; repository evidence is only overwritten when explicitly selected.

check-modules was additionally executed and failed due to the existing shared-version policy (v0.0.0 versus v0.30.0 and latest root tag v0.22.0). version-baseline.log captures HEAD config/viper/go.mod and the latest root tag. Exact failure output is darwin-arm64/check-modules-deferred.log. E6 release work remains deferred; make all is not claimed to pass. No workflow/manifests/version sync were modified.

Related dependency inspection: `(cd http && GOWORK=off go list -m golang.org/x/time)` selected v0.12.0; resty/v2 v2.17.2's go.mod/test imports explain the legacy transitive graph. No new dependency is added to ratelimit. See legacy-time-dependency.log and inventory.

## Linux ARM64

Image sha256:e432b43af23a9328d56a7c499be0476810aa344acbcf65fc7c455d4ff5a40602. Flags: --network none --read-only --cpus 4 --memory 4g --tmpfs /tmp:rw,exec,size=3g. Repository read-only at /src; only evidence/linux-arm64 writable at /results. PATH=/tmp/go/bin:$PATH, GOCACHE=/tmp/cache, GOMODCACHE=/tmp/modcache, RATELIMIT_RESULTS_DIR=/results.

Existing /private/tmp/config-linux-validation.tar.gz supplied Go1.27.4/cache through stdin (`tar -xz -C /tmp`). Official Linux ARM64 toolchain archive SHA256: 3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92. New module and consumers use only stdlib and selected local modules.

```sh
/src/scripts/check-ratelimit-production.sh all
/src/scripts/check-ratelimit-module.sh > /results/consumers.log 2>&1
```

## Compatibility and diagnostics

Copied go.mod and *.go into a temporary directory outside Git; externally installed /private/tmp/go-engine-tools/gorelease -base=none -version=v0.1.0 exited 0. First version, no historical API comparison, fabricated tags or private proxy. Tool dependency is outside ratelimit/go.mod.

From ratelimit: `GOWORK=off go test -coverprofile=evidence/darwin-arm64/coverage.out ./...` and `go tool cover -func=...`. Coverage is diagnostic, not acceptance proof. Timer.Stop/Reset behavior was verified with local go doc; output in timer-api.log. There is no legacy limiter benchmark comparison because no equivalent implementation was found.

## Hash verification

Repository root: `shasum -a 256 -c ratelimit/evidence/integration-sources.sha256`.
ratelimit: `shasum -a 256 -c SOURCE.sha256`.
ratelimit/evidence: `shasum -a 256 -c SHA256SUMS`.

Source manifest covers module code/tests/docs. Integration manifest covers root integration edits, scripts and consumers. Evidence manifest excludes itself. No staging, commit, push or tags.
