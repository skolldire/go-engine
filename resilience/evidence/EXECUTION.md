# Execution record

Initial HEAD 5762d7f (ratelimit), feature/split, porcelain empty. Go1.27.4 darwin/arm64 (Apple M1 Max) and linux/arm64 (Colima container). No AMD64 run. Root regression/lint, comparison and gorelease attempt ran on macOS. Linux ran resilience qualification plus both consumers, not the full legacy repository.

## Native commands

```sh
export PATH=/private/tmp/go-engine-toolchain/go/bin:/private/tmp/resilience-tools/bin:$PATH
export GOCACHE=/private/tmp/go-engine-build-cache GOPROXY=off
export GOMODCACHE=/private/tmp/go-engine-mod-cache
export RESILIENCE_RESULTS_DIR="$PWD/resilience/evidence/darwin-arm64"
./scripts/check-resilience-production.sh all
export GOMODCACHE=/Users/seven/go/pkg/mod
export GOLANGCI_LINT_CACHE=/private/tmp/go-engine-lint-cache
make lint
make lint-arch lint-docs lint-deps check-resilience check-ratelimit check-circuitbreaker check-retry check-client check-config check-concurrency
make test
go test -race -count=1 ./pkg/utilities/resilience/... ./pkg/core/client/...
make check-modules
go test -run '^$' -bench '^BenchmarkBoundarySuccess$' -benchmem -benchtime=500ms -count=3 ./pkg/utilities/resilience
(cd resilience && GOWORK=off go test -run '^$' -bench '^BenchmarkBoundarySuccess$' -benchmem -benchtime=500ms -count=3)
```

The final complete qualification followed the cause-boundary correction, retained regression seed and additional typed-error/recovery tests. Final logs replace exploratory runs; findings are recorded in RELEASE_NOTES. No base modules or dependency versions changed. The linter symlink pointed to golangci-lint 2.14.0. Root tests needed permission for existing local HTTP/gRPC listeners; the new module and consumers use no sockets. Existing dependency cache-stat warnings under sandbox guards were nonfatal.

Production script sets GOWORK=off/GOTOOLCHAIN=local/GOPROXY=off/GOFLAGS=-mod=readonly. Quick runs race/count=20/timeout=120s at default GOMAXPROCS, 1 and 4, then vet. Stress: RESILIENCE_STRESS=1/race/count=3/timeout=180s. Config/Sequences/Budgets fuzz: 30s each, two workers. Benchmarks: 500ms/count=3, without race; RunParallel uses PB.Next. RESILIENCE_RESULTS_DIR defaults to a new temporary directory; output is reproducible without overwriting repository evidence implicitly.

check-modules failed under the existing shared-version policy: v0.0.0 versus v0.30.0 and latest root tag v0.22.0. version-baseline.log captures HEAD config/viper requirement and tag; exact failure in darwin-arm64/check-modules-deferred.log. No manifest/version synchronization/workflow edits, and make all is not claimed to pass.

## Linux ARM64

Image sha256:e432b43af23a9328d56a7c499be0476810aa344acbcf65fc7c455d4ff5a40602, --network none --read-only --cpus 4 --memory 4g --tmpfs /tmp:rw,exec,size=3g. Repository read-only at /src; evidence/linux-arm64 writable at /results. PATH=/tmp/go/bin:$PATH, GOCACHE=/tmp/cache, GOMODCACHE=/tmp/modcache, RESILIENCE_RESULTS_DIR=/results.

The existing /private/tmp/config-linux-validation.tar.gz supplied Go1.27.4/cache through stdin (`tar -xz -C /tmp`). Official Linux ARM64 archive SHA256: 3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92. This module's graph needs only local base modules and stdlib.

```sh
/src/scripts/check-resilience-production.sh all
/src/scripts/check-resilience-module.sh > /results/consumers.log 2>&1
```

## Compatibility tool and coverage

Copied go.mod/*.go for resilience and the five siblings into a fresh temporary directory outside Git (and resilience/testdata). Ran externally installed /private/tmp/go-engine-tools/gorelease -base=none -version=v0.1.0 from its resilience directory. Exit 1: gorelease ignores local replaces in its dependency loading and circuitbreaker@v0.0.0 cannot be resolved offline. Exact output in darwin-arm64/gorelease.log. E5 remains pending, not passed. No fabricated tags/proxy or historical comparison. Tool installation is outside module go.mod.

From resilience: GOWORK=off go test -coverprofile=evidence/darwin-arm64/coverage.out ./..., then go tool cover -func=.... Coverage is diagnostic, not acceptance evidence by itself.

## Graph and hashes

module-graph.log records go list -m all and go mod graph: resilience plus five base modules, no SDK. Both minimal/composed consumers resolve those six libraries (plus their main module), even with inactive tools. Each consumer replaces all six explicitly; dependency replaces are not inherited. Guards enforce exact module/package/graph/replace allowlists and inverse import boundaries.

Repository root: shasum -a 256 -c resilience/evidence/integration-sources.sha256.
resilience: shasum -a 256 -c SOURCE.sha256.
resilience/evidence: shasum -a 256 -c SHA256SUMS.

Source manifest includes the retained fuzz regression seed. Integration manifest covers root edits, guards/consumers and test-only legacy benchmark. Evidence manifest excludes itself. No staging, commit, push or tags.
