# client/otel candidate — D46 approved

## C1: initial state and contract before code

Branch feature/split, HEAD3d317bdc849be1b43576bc4373e80bbd736e596e. Initial git status --porcelain:

```
 M CHANGELOG.md
 M MIGRATION.md
 M go.work
 M modules.tsv
?? examples/standalone/telemetry/
?? telemetry/
```

These are prior telemetry/otel D45 candidate changes, preserved and not attributed to D46. Shared files receive only separate client/otel registration/documentation additions. No index modification, commit, push, tag, release workflow or legacy production change is authorized. Files containing " 2" are excluded. Go1.27.4 darwin/arm64. Root go list -m confirms OTel API/trace/metric/sdk/sdk-metric v1.47.0; existing directives/versions remain unchanged.

D46/D46.1–D46.6 are approved. D35/D43/D45.8 remain unchanged. Public API: Config{InstrumentationName string}, DefaultConfig (empty required name), Validate, Dependencies{TracerProvider trace.TracerProvider; MeterProvider metric.MeterProvider; IsFailure func(error)bool}, New[T](Config,Dependencies)(client.Middleware[T],error), ConfigError{Field,Reason} wrapping ErrInvalidConfig. Config tags json/mapstructure snake_case. No own logger/provider/SDK abstraction or lifecycle.

Validation order: Config name; typed-nil pointer TracerProvider; typed-nil pointer MeterProvider; both absent; IsFailure without tracing. No trimming/charset/length restriction added. Reflect nil detection only in New. New calls supplied providers to obtain native API instruments, but starts no span/measurement, timers or goroutines. One Float64Histogram go_engine.client.operation.duration, unit s; no calls counter or partial-instrument registration case (D46.3 supersedes proposal wording). Instrument creation error wraps its cause with fixed context, no false ErrInvalidConfig.

Span name Component.Operation, Kind Internal (D46.1); own attributes go_engine.client.component, go_engine.client.operation, go_engine.client.success (D46.2). Parent transport instrumentation can create a Client child, avoiding duplicate Client logical spans. Tracing passes Start's context to next; metrics-only passes the exact caller context. Values/deadline/cancel/cause and results/errors are preserved. Histogram measures only next with one monotonic clock, not start/finalization callbacks. Sampling never disables metrics.

No automatic error status or RecordError. IsFailure runs once for a nonnil operation error, before End and outside locks; true requests codes.Error with empty description, false/nil leaves status alone. Native Ok>Error>Unset precedence remains; the API cannot read existing status. IsFailure panic propagates, closes the span, and emits no normal-completion measurement. Caller-owned backend callbacks may panic/block/Goexit and prevent normal return; no recover or helper goroutine hides them.

Normal return adds success, closes span once, then records histogram with the span context retained. Panic/Goexit closes span through a deferred closure calling End, never direct defer span.End. No success attribute or histogram is fabricated for abnormal completion. Installed SDK trace/span.go End (~403–440) calls recover directly and records exception.message then re-panics only when End itself is the deferred function; an extra closure makes that recover non-direct and prevents this SDK event path. Harness must prove default-SDK behavior and same panic identity. No provider option such as WithoutPanicRecording is required.

Dependencies are borrowed, never closed/configured. Names/tags are static/low-cardinality/nonsecret by caller responsibility. No result/error text, payload, IDs, stacks, baggage extraction or automatic propagation. Resource/environment merge, sampler, limits, views, exemplars and metric cardinality belong to the provider. API-only applies to module tests too; real SDK tests live in a nested nonpublished harness.

## Inventory and migration map

| Origin | Destination | Action | Required evidence |
|---|---|---|---|
| pkg/core/client/observability.go WithTracing/Tracer.Span | API-only New[T] tracing | adapt | native Start/context/span naming/parenting |
| pkg/core/client/observability.go WithMetrics/Recorder.RecordOperation | one native duration histogram | adapt | count equals normal completions, seconds, labels |
| pkg/core/client/middleware.go Invocation.Client / any | client.Invocation.Component / T | adapt, no alias | typed partial values/error identity |
| pkg/core/client/entity.go BaseClient.Use mutable chain and timeout | explicit client.Do/Chain | exclude ownership | no timeout/mutation/revalidation |
| pkg/core/client/middleware_test.go | legacy suite unchanged | preserve | root regression |
| pkg/telemetry/otel/telemetry.go Span | no facade dependency; optional IsFailure | deliberate change | no RecordError/error text/automatic Ok |
| telemetry.go dynamic instrument caches and log warnings | fixed histogram created in New | exclude cache/logger | construction errors wrapped, no history |
| pkg/telemetry/otel/middleware.go otelhttp | later transport instrumentation | exclude | no HTTP/transport dependency |
| examples/standalone/telemetry/otel/composed/observer | reference only, not imported/migrated | preserve prior work | SDK harness checks closure-based End |
| telemetry/otel provider/resource/shutdown | caller assembly | exclude from module | API-only guard, SDK integration harness |

Legacy tracing/metrics decorators are optional and nil is a no-op, return any and preserve the next result/error. Metrics measures time.Now/Since and calls Recorder only on normal return; tracing delegates name Client.Operation to a supplied facade, which can change error behavior. Actual telemetry facade Span automatically RecordError, uses err.Error for status, marks success Ok and directly defers End; its SDK can record panic content. The new wrapper does none of these automatically. Existing WithTracing/WithMetrics direct references found in observability.go and middleware_test.go; BaseClient and its family consumers are unchanged. Legacy facade also owns or borrows providers, caches arbitrary instruments in sync.Map and logs creation failures; those responsibilities do not migrate here.

## D46.6 checker proposal and approval

The original generic checker permitted composed catalog tools but not SDK modules directly imported by this application's processor/ManualReader assembly. The user approved scripts/modules/consumer-deps.tsv with columns module-dir, composed, exact external module paths and the six conditions recorded below. The implementation extends only composed boundaries; module/minimal remain unchanged. Mutation coverage rejects SDK imports in wrapper/minimal and undeclared composed, accepts declared composed, and preserves exact replaces/no-Logrus policy.

## Increment evidence and acceptance

C1 inventory and approved contract recorded before code. C2 configuration/errors/validation pass. C3 provider callbacks and one histogram construction pass, preserving creation causes. C4 typed results/context/one-clock duration pass. C5 normal/abnormal finalization, classifier/backend panic and ordering tests pass. C6 API-only stress/leaks/fuzz/bench implemented and executed. C7 real SDK harness, independent consumers, composed-only data guard and mutation proof pass. C8 versioned documentation, root references and evidence summaries are included. E1–E4 results and E5 distribution impediment are detailed below; E6 proposed independent v0.1.0 remains unapproved, catalog unreleased/-; E7 real-tag consumers deferred. No stability/publication claim.

D46.6 extension approved by the user with six conditions: exact external module paths only (no wildcards/repo modules); composed only; bidirectional checking against consumer go.mod and actual direct use; missing module/dir rejected; D44/global rules retained; mutation cases SDK in wrapper/minimal/undeclared composed fail, declared composed passes, unused/logrus declarations fail. Schema and generic validation will live in scripts/modules/consumer-deps.tsv and the shared reader/checker, not a module-specific script.

## Executed evidence and reproduction

Executed 2026-11-13 using Go1.27.4 on darwin/arm64 (Apple M1 Max) and linux/arm64 (local disposable container,4 CPUs/4GiB, network disabled). No AMD64 execution is claimed. Root lint/regression/tooling mutations ran on macOS only; Linux scope is the wrapper/harness/standalone consumers. golangci-lint2.14.0 was already available externally. No existing Go directive, API/SDK version or root go.mod/go.sum changed. The offline cache needed the existing root-selected yaml/v3 v3.0.6 rather than SDK/API minimum3.0.5; prepared sums resolved an initial sumdb network lookup failure without disabling verification or upgrading dependencies.

Module commands use GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly with a prepared cache. Generic commands set these themselves. From root:

```sh
scripts/modules.sh validate
scripts/modules.sh lint-lists
scripts/check-module.sh client/otel deps
scripts/check-module.sh client/otel consumers
scripts/check-module.sh client/otel quick
scripts/check-module.sh client/otel stress
CLIENTOTEL_FUZZ_TIME=3s scripts/check-module.sh client/otel fuzz
CLIENTOTEL_BENCH_TIME=100ms scripts/check-module.sh client/otel bench
scripts/check-module.sh client/otel/integration deps
scripts/test-modules.sh consumer-deps
scripts/test-modules.sh unit
scripts/test-modules.sh decisions
make lint
make lint-arch lint-docs check-modules
make test
scripts/check-module.sh client consumers
scripts/check-module.sh telemetry/otel consumers
scripts/check-module.sh resilience consumers
```

All listed checks passed on macOS. The root regression includes legacy core/client and pkg/telemetry/otel/provider tests and all catalog modules, including the new harness. Local test listeners required sandbox permission; no external service was used by the new suites. Two intermediate harness lint issues (tagged-switch suggestion and formatting/time expression in an additional benchmark) and offline sum preparation were corrected before accepted runs; no failed attempt counts as approved.

Quick executes race/count20 with default GOMAXPROCS,1,4 and vet. Both wrapper and harness passed that matrix on both ARM64 platforms. The harness commands, run from client/otel/integration with the offline environment above:

```sh
go test -race -count=20 ./...
GOMAXPROCS=1 go test -race -count=20 ./...
GOMAXPROCS=4 go test -race -count=20 ./...
go vet ./...
CLIENTOTEL_STRESS=1 go test -race -run '^TestStress' -count=3 -v ./...
go test -run '^$' -bench . -benchmem -benchtime=100ms -count=3 ./...
```

SDK tests use default panic-recording-enabled TracerProvider, without WithoutPanicRecording, and prove End through closure exports no exception event for panic/Goexit and preserves the exact panic value. IsFailure panic also closes without a normal histogram. Both direct SDK and telemetry/otel providers pass parenting, Kind Internal, exact namespace, status precedence, histogram seconds/count and sampled exemplar correlation. Unsampled calls still contribute histogram observations. No SDK policy, lifecycle or globals are added to the wrapper. These are native SDK checks, not simulations in the API-only tests.

Stress/count3: wrapper exercises96,000 calls per run across trace-only/metric-only/both, alternating results and canceled contexts; total288,000 calls. Additional200 caller panics per run close spans and emit no completion histogram. Each memory run abandons4,000 configured16KiB closure fixtures after calls finish. Warmed post-GC macOS heap pairs299,184→299,456,316,272→316,272,338,880→338,880 bytes; fixture threshold8MiB allows runtime/API caches, not a general process memory promise. SDK harness contributes16,000 shared calls per run,48,000 total, with equal span/histogram counts, plus2,500 short-lived wrappers per memory run. SDK heap pairs341,544→335,152,357,280→350,648,363,328→356,712 bytes. Standard goroutineleak reports total0. API-only doubles have bounded test-owned capture; module code has no per-call history.

Three fuzz targets at3s/parallel2: macOS Config62,960, LabelsAndResults148,551, ReuseSequence88,683 executions; Linux Config63,719, LabelsAndResults154,605, ReuseSequence83,622. All passed. Arbitrary label bytes obey client.Do's existing length contract, without UTF-8 normalization or restrictions added by the wrapper. Fuzz/stress are bounded campaigns, not exhaustive claims about arbitrary callbacks/vendors. Coverage98.3% is diagnostic only.

Benchmarks use b.Loop/benchmem and RunParallel/PB where appropriate,100ms/count3. API no-op baseline, direct call, client.Do, construction, trace/metric/both, reused chain, constructing chain, struct result and shared concurrency all ran on both platforms. Native SDK harness measures sampling on/off with ManualReader, a counting processor without unbounded stored history, and equivalent direct API Start/attributes/End/histogram calls. The additional equivalent-API/counting-processor benchmarks ran separately on both platforms after the initial campaign; no production behavior changed. A span-only reference is explicitly not equivalent to the full wrapper+histogram. Representative macOS both-signals API-noop816B/11allocs; sampled SDK wrapper1808B/15allocs versus equivalent direct API1792B/14allocs. Costs include the declared fixture and chain choice; no SLA, universal overhead delta or zero-allocation promise is made.

Linux used image e432b43af23a with official Go1.27.4 linux-arm64 archive and the prepared selected-module cache transferred via stdin into tmpfs; source snapshot/toolchain/cache were disposable and network disabled. It executed both race matrices, both vets, both stress/count3 campaigns, wrapper fuzz targets, all benchmark fixtures and both consumers go run/go test-race. Root lint, root make test and generic checker mutation campaigns are macOS-only evidence. No deployment/collector/network container was introduced.

D46.6 guard proof: declared composed passes;12 negative mutations in a disposable current-tree snapshot reject SDK in wrapper, SDK in minimal, undeclared SDK composed, unused external declaration, declared Logrus, minimal entry, module entry, wildcard entry, repository-module entry, unknown owner, missing go.mod require and missing composed directory. Default, independent and unreleased graph checks still enforce D44. Existing unit/matrix and D43/D44 import/Logrus mutations passed. No mutation touched the live tree/index. Source imports are parsed across build tags; helper/metadata validation uses only stdlib and offline Go. Bash syntax and existing Bash3.2-compatible conventions are retained, with no jq/Python/yq/new per-module script.

Wrapper dependency graph:13 selected modules including main,152 build/test dependency packages. Only client and OTel API modules are direct; no provider SDK/SDK-metric/factory/exporter package is imported in production or tests. Minimal also has no provider SDK or telemetry/otel. go.opentelemetry.io/auto/sdk is a legitimate API transitive, not go.opentelemetry.io/otel/sdk. SDK metadata may refer to its own test dependencies in the harness graph; no external testing library is imported here. Composed explicitly declares SDK and SDK-metric as application data and uses real telemetry/otel+resilience+retry; one successful logical call encloses three attempts, and a second uncertain publication is Reconcile without repetition. The histogram has two observations. Replaces point only at used catalog modules and generic graph/import/replace checks pass offline.

Gorelease E5: copied the wrapper and its local client sibling to a temporary directory outside the worktree and invoked the externally installed tool there, GOWORK=off/GOPROXY=off, `-base=none -version=v0.1.0`. It could not resolve github.com/skolldire/go-engine/client@v0.0.0 (module lookup disabled by GOPROXY=off). This is the expected unresolved unpublished-sibling distribution impediment, not a compatibility approval. First version, no published base; no historical comparison, tags, proxy or replacements pretending publication were created. Harness is track none and is not a release target.

Raw logs/graphs/coverage are local ignored evidence/, not claimed as versioned deliverables. This versioned summary preserves commands, workload/time bounds, observed results and limitations. EVIDENCE.sha256 identifies local evidence; SOURCES.sha256 identifies candidate files, excluding the two manifests. Key logs: quick-final.log, stress/stress.log, fuzz/fuzz-Fuzz*.log, bench/benchmarks.log, harness-race-*.log, harness-stress.log, harness-bench*.log, mutations.log, root-lint.log, root-guards.log, root-test.log, related-consumers.log, gorelease.log, linux-arm64.log and linux-bench-equivalent.log.

| Evidence | Result |
|---|---|
| E1 | D46 and six precisions recorded before code; README/MIGRATION/CHANGELOG/contract and generic consumer data documented. |
| E2 | API-only unit/race/resource/stress/fuzz/bench passed on both ARM64 platforms within the scopes above. |
| E3 | Real SDK harness passed native parenting/status/histogram/sampling/panic/privacy checks with no external services. |
| E4 | API-only module/minimal and composed real tools pass offline; generic guard's positive and12 mutation cases pass on macOS. |
| E5 | Gorelease attempted externally; blocked by unpublished client@v0.0.0, first version without base. |
| E6 | Independent v0.1.0 is proposed, not approved; catalog stays unreleased/-. |
| E7 | Real-tag consumers deferred until separately authorized publication and resolvable sibling tags. |

Remaining limits: caller/backends own concurrency safety, cardinality, sampling/resource/limits/export/lifecycle, error classification and callback behavior. No universal redaction or delivery guarantee; callback panic/Goexit can prevent a completed I/O return. No automatic metrics for abnormal completion. No root validation on Linux or AMD64 run is claimed. D45 work remains separate; its sources and prior changes were preserved. No commit/push/tags/index changes or release workflow execution occurred.

Final benchmark completion: prebuilt Chain without middleware and independent per-worker middleware instances were additionally measured on macOS and Linux ARM64 (100ms/count3), with the module race suite and lint rerun after adding those fixtures. These are benchmark-only additions; production code stayed unchanged after the approved implementation. Final catalog validation, lint-lists, deps/consumers, documentation guard and whitespace checks pass; root go.mod/go.sum and Git index remain unchanged from the initial state.
