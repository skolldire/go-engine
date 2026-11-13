# client/logging candidate

## Initial state and approved contract (C1)

Branch feature/split, HEAD70a10ac81a2f6626f167b75af9309af362ac9adc. Initial git status --porcelain: empty. Go1.27.4 darwin/arm64. No prior work is attributed to this task. D42 and D42.1–D42.4 are approved as communicated by the user.

D32 resolution: the former concrete *slog.Logger requirement is replaced by a consumer-side Logger interface with only Enabled(context.Context,slog.Level)bool and LogAttrs(context.Context,slog.Level,string,...slog.Attr). *slog.Logger satisfies it; direct implementations need no slog.Handler. The local public-contracts document, if present, is corrected before implementation. This supersedes the old concrete dependency without changing client itself.

Public API: Config{Level string, IncludeDuration bool} with mapstructure/json snake_case tags; Logger interface above; Dependencies{Logger Logger}; DefaultConfig() Config; (Config).Validate()error; New[T](Config,Dependencies)(client.Middleware[T],error); ErrInvalidConfig and ConfigError{Field,Reason}, Unwrap to the sentinel. No other public entry points.

Defaults: debug and IncludeDuration=true. Only debug/info, no normalization or implicit defaults. Validate Config before Logger. Nil interface and specifically *slog.Logger(nil) fail with ConfigError.Field Logger. Other implementations' typed nil values are not universally detected. New is pure and captures Config by value and a borrowed Logger; no Enabled, emission, I/O, clock reads, timers, context derivation, goroutines or global mutations at construction.

Execution: middleware(nil) returns nil. Otherwise Enabled is queried once on entry; disabled calls go straight to next without clock/attrs/emission. Enabled normal returns emit once using the fixed message operation completed and attributes component, operation, success=(err==nil), plus optional duration. Duration covers next but not final logging, using monotonic Now/Since. The same ctx/info/operation reach next and original result/error return unchanged. No payload/result/error string, metadata extraction, validation duplication, severity escalation or retries. Context cancellation is not an automatic rejection.

Panic/Goexit from next have no completion record and propagate without recovery or deferred logging. Logger panics propagate too, including after an operation has already completed; backend errors ignored by its Logger implementation do not replace I/O results. No durable-delivery promise. A dynamic *slog.Logger may filter again at LogAttrs; one emission attempt is not guaranteed physical delivery. User loggers/handlers/next must be concurrency-safe; dependencies are borrowed and never closed. Client.Do validates; prebuilt handlers assume their caller's preconditions.

## Inventory and migration map

| Origin | Destination | Treatment | Required test |
|---|---|---|---|
| pkg/core/client/observability.go WithLogging | New[T] middleware | adapt; two messages become one normal completion | exact event/count/order |
| observability.go logger.Service dependency | Logger interface | replace with two consumer-owned methods | JSON slog + own slice logger + disabled logger |
| entity.go NewBaseClient and IsLoggingEnabled/SetLogging | caller-selected middleware + Logger.Enabled | no mutable enabled callback in this module | pure New, runtime filtering |
| middleware.go Invocation.Client | client.Invocation.Component | distinct typed API, no alias | stable labels, Invocation unchanged |
| observability.go error branch | success=false at fixed level | exclude err.Error and promotion to error | poisonous error, unchanged identity |
| observability.go any result passthrough | generic T passthrough | preserve values without formatting | pointer/slice/map/struct + partial result |
| observability.go panic/cancel behavior | normal-return-only observation | explicit propagation, no finalization on abnormal return | panic/Goexit, canceled ctx |
| examples/standalone/logging/composed local observe | independent public middleware | reference implementation only, leave consumer untouched | new composed uses real module |
| legacy WithMetrics/WithTracing | later wrappers | exclude | graph only client/stdlib |

The legacy wrapper logs Debug start before next, then Debug success or Error with the returned error text. Nil logger/disabled callback bypass it. Its next call returns the original result/error, and a panic skips the success/failure log after already logging start. BaseClient installs the wrapper and consults an RWMutex-protected logging flag per invocation. Those flags do not make arbitrary supplied loggers or next handlers safe. Existing consumers include pkg/core/client/entity.go, http/pkg/rest/{entity,service}.go and messaging/pkg/integration/grpc/{entity,service}.go via the legacy BaseClient; their tests and AWS client tests exercise legacy logging behavior. None is migrated or modified.

## Increment evidence

C1: clean preflight, inventory and approved contract recorded before implementation.
C2: config validation, nil handling, pure construction, nil terminal and private clock-spy paths pass.
C3–C5: original arguments/results/errors, filtering, duration in virtual time, order, panic/Goexit and concurrency pass unit/race; lint reports zero issues.
C6: quick race count20/default/1/4 + vet, stress count3, four15s fuzz targets and 500ms/count3 benchmarks pass on macOS ARM64 and Linux ARM64. Standard goroutineleak passes; coverage100% is diagnostic, not proof of correctness.
C7: guards and both offline consumers pass; minimal has native slog + own Logger, composed uses real logging/resilience and tools. Existing client/logging-factory/resilience consumers pass before composition.
C8: README/MIGRATION/CHANGELOG/root references complete; final guards, source/evidence hashes and limitations recorded. No version approved or published.

## Acceptance

| Evidence | Status | Scope |
|---|---|---|
| E1 contract/documentation | passed | D42 and four clarifications; concrete dependency replaced under D32 before code |
| E2 functional/resources | passed | race20/default/1/4, vet, stress3, leaks, virtual time, fuzz and benchmarks on both ARM64 platforms |
| E3 integration | passed | memory-backed transport/writers, partial results, cancellation, middleware order and handler failures |
| E4 independent consumers | passed locally | three Logger implementations; minimal excludes logging/resilience; composed uses all real tools |
| E5 gorelease | blocked/pending distribution | external tool attempted -base=none -version=v0.1.0; client@v0.0.0 cannot resolve offline after local replaces are ignored |
| E6 version approval | pending | proposal client/logging/v0.1.0 only; actual track unreleased, version - |
| E7 real-tag consumers | deferred | separately approved/resolvable dependency and module tags required |

No commit, push, tags, proxy fabrication or release workflow.


## Executed commands and platforms

Go1.27.4 darwin/arm64 (Bash3.2, existing golangci-lint2.14.0) and linux/arm64 (Bash5.2). No AMD64 claim. Local toolchain: /private/tmp/go-engine-toolchain/go/bin; GOCACHE=/private/tmp/go-engine-build-cache, GOMODCACHE=/Users/seven/go/pkg/mod. External tool caches are not module dependencies. No Go directive/dependency of an existing module was changed.

Reproduce from root with Go1.27.4 on PATH:

```sh
scripts/modules.sh validate
scripts/modules.sh lint-lists
scripts/check-module.sh client/logging deps
scripts/check-module.sh client/logging consumers
CLIENTLOGGING_RESULTS_DIR="$PWD/client/logging/evidence/local" \
CLIENTLOGGING_FUZZ_TIME=15s CLIENTLOGGING_BENCH_TIME=500ms \
scripts/check-module.sh client/logging all
make lint
make lint-arch
make lint-docs
make check-modules
make test
for m in client logging resilience; do
    scripts/check-module.sh "$m" consumers
done
```

Generic quick runs GOWORK=off go test -race -count=20 -timeout=120s ./... three times (default GOMAXPROCS,1,4), then go vet. Stress uses CLIENTLOGGING_STRESS=1/CLIENTLOGGINGSTRESS=1, TestStress, -race and count3. Fuzz each target15s, two workers. Benchmarks500ms/count3 without race, b.Loop and RunParallel/PB.Next. Default longer campaigns remain available through the generic checker. Disabled/private clock spies verify no measurement, while synctest validates duration3s excluding7s emission and logical duration4s across two1s attempts plus2s backoff. No precise real sleeps or external service dependencies.

Final fuzz counts:

| Target | macOS ARM64 | Linux ARM64 |
|---|---|---|
| FuzzConfiguration | 797072 | 700162 |
| FuzzInvocation | 829714 | 696087 |
| FuzzSequences | 628339 | 543085 |
| FuzzComposition | 807267 | 735289 |
| Total | 3062392 | 2674623 |

All pass; finite campaigns do not establish correctness for every possible input. Stress each repetition:24 callers/9600 completions,4800 original errors unchanged; disabled independent wrappers also pass. Retention fixture:800 completed chains with64KiB context payloads, eight rounds/GC. macOS baselines291–324KiB and final288–321KiB remain within the fixture-specific8MiB allowance; no heap bound on arbitrary backends/payloads is claimed. Standard runtime/pprof goroutineleak is asserted empty; every helper goroutine/gate terminates.

Representative macOS medians with fixed memory/discard fixtures: direct operation2.16ns/op; client.Do13.9ns/op; prebuilt disabled wrapper6.13ns/op; enabled/no-duration58.2ns/op; enabled/duration109ns/op; full standard JSON backend780ns/op. Enabled attribute passing costs160B/1allocation in those chain fixtures; disabled observed0B/0allocations. These are local measurements, not zero-allocation/throughput/SLA promises. Construction, reused Chain vs Do, scalar/struct results, standard slog reference and shared/independent JSON backends are separate benchmark cases. slog.DiscardHandler is a disabled standard backend control, not serialization cost. No legacy speed comparison is claimed: legacy emits two records and formats errors, so it is not an equivalent fixture; no root test was added for a false comparison.

Root macOS lint/architecture/docs/version/catalog checks and make test across the workspace pass. Regression consumers of client, logging and resilience pass before and after the new composition. Existing legacy source/tests/consumers, Makefile, workflows and scripts are unchanged. The root regression ran with local-port permission required by existing httptest/gRPC/Redis fixtures; no such service is needed by the new module or its consumers.

Linux execution uses the existing local ARM64 image e432b43af23a with4 CPUs/4GiB, network disabled, /src read-only and only the result directory writable. The existing Go1.27.4 Linux toolchain is injected via stdin into tmpfs; no download or service container is required. Linux ran catalog/list/dependency/consumer checks and the full module campaign; root lint/workspace regression are macOS-only, not attributed to Linux.

## Independence and backend evidence

Module graph: client/logging + client only; no require or import of our logging factory, other tools or SDKs, including tests. Parent client remains stdlib-only. The catalog row is composition/unreleased/-/production/deps client; go.work includes the new module. No per-module script or duplicate allowlist is introduced.

Minimal selected modules: its own harness, client/logging and client. It executes both native slog JSON and a direct application slice Logger with no slog.Handler. Unit tests additionally execute a disabled implementation and private clock spy, fulfilling D42.4 without a backend import.

Composed selected modules: its harness plus client/logging, client, logging, resilience, retry, ratelimit, concurrency and circuitbreaker. It uses real APIs and five-level local replaces. Three transient attempts produce one logical completion; an uncertain operation returns partial99 plus the original uncertain/reconciliation causes without repeating. Public limiter acquisition verifies released permits; breaker stats see4 actual attempts and zero in-flight. A separate synctest checks the logical duration includes retry/backoff. No private error text enters the event.

All consumers run GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly. Imports, modules, go mod graph and exact local replaces are checked by the catalog-derived guard. graph-module.log, graph-minimal.log and graph-composed.log record the actual selected modules/edges/imports. Local replaces prove independence of the framework, not installability against nonexistent/unapproved tags.

## Gorelease and remaining distribution gates

External binary /private/tmp/go-engine-tools/gorelease was invoked outside the module's go.mod in a disposable copy preserving the real client/child layout, with -base=none -version=v0.1.0, GOWORK=off and GOPROXY=off. It exits1 while resolving client@v0.0.0, reporting module lookup disabled; its -d deprecation warning is not the blocking cause. The tool does not use the local development replace for its release dependency resolution. No network lookup of a published client version is claimed.

This is the first candidate without a published comparison base. E5 is not passed and no historical compatibility report is invented. Once versions/dependency tags are approved and resolvable, replace the development requirement as part of that authorized distribution task, rerun genuine gorelease and the consumers against the real tag. No tags or artificial proxy/cache entries were created here.

## Artifacts and boundaries

Versionable documentation records the contract, decisions, commands, results and limitations. Source manifest client/logging/SOURCES.sha256 verifies from root with shasum -a256 -c client/logging/SOURCES.sha256. Local ignored logs live under client/logging/evidence/{darwin-arm64,linux-arm64}; SHA256SUMS is relative to that evidence directory. Those logs are not represented as versioned deliverables.

Backend adapters in README not executed here (Logrus bridge, zap/zapslog, OTel/otelslog) are illustrative and versionless. This module does not implement or claim their integration. A backend may filter again, retain records, block or panic; labels/backend-owned fields remain the caller's confidentiality responsibility. Config defaults to debug, so an info-threshold backend filters it unless the application selects info or adjusts the backend. No client, logger factory, legacy implementation or resilience guarantee was changed.
