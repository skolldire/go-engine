# telemetry/otel candidate

## Initial state and D45 contract before code (C1)

Branch feature/split; initial HEAD3d317bdc849be1b43576bc4373e80bbd736e596e; git status --porcelain empty. Go1.27.4 darwin/arm64. Root go list -m confirms OTel API/trace/metric/sdk/sdk-metric all v1.47.0; no dependency/directive upgrade is authorized. D45/D45.1–D45.8 are approved. D32 supersedes the old InstallGlobal/managed-processor/byte-limit contract before implementation.

Approved clarification from the user during this task: SDK v1.47.0 does NOT enforce value length in events/links/exceptions. Keep native providers without facades; MaxAttributeValueLength applies to span attributes (Start/SetAttributes/context access) only. Events/links keep count/depth limits but no value-length promise, including RecordError's exception event. Evidence sdk-limits-proof.log reproduces a one-character limit: span values become one rune/three bytes, event/link values remain three runes/nine bytes and exception.message remains22 characters. The initial architectural premise is explicitly corrected, not silently claimed.

Public API: Config{ServiceName,ServiceVersion,Environment string; SamplingRatio float64; MaxAttributes,MaxAttributeValueLength int}, mapstructure/json snake_case tags; DefaultConfig/Validate; Dependencies{TraceProcessor sdktrace.SpanProcessor; MetricReader sdkmetric.Reader}; New(Config,Dependencies)(*Provider,error); Tracer(name) trace.Tracer; Meter(name) metric.Meter; TracerProvider() trace.TracerProvider; MeterProvider() metric.MeterProvider; Shutdown(context.Context)error; ConfigError{Field,Reason} wrapping ErrInvalidConfig; ErrNilContext. ErrInvalidCall is removed under D45.2 because no actual error path uses it. No InstallGlobal, closed/global sentinels, extra configuration or SDK concrete getters.

Defaults: empty required ServiceName, optional version/environment empty, SamplingRatio1, MaxAttributes128, MaxAttributeValueLength4096. Literals validated unchanged in declaration order; sampling finite0..1; limits positive. Service identity strings are not silently truncated or subjected to new undocumented length/charset restrictions. Resource NewWithAttributes with semconv/v1.43.0.SchemaURL, service.name, optional service.version/deployment.environment.name. No detectors/default resource merge in module code.

Native SpanLimits: WithRawSpanLimits, count/per-event/per-link counts=MaxAttributes; string value length=MaxAttributeValueLength (SDK code points, NOT bytes); event/link count128 each; SDK-native AttributeValueDepthLimit64 explicitly fixed. No wrappers around tracers/spans. Sampling ParentBased(TraceIDRatioBased(ratio)); ratio governs roots, inherited sampled parents can record with ratio0. Metrics use native SDK readers with no module cardinality policy: WithCardinalityLimit(0) prevents OTEL_GO_X_CARDINALITY_LIMIT/default2000 from silently adding one. WithExemplarFilter(TraceBasedFilter) fixes OTEL_METRICS_EXEMPLAR_FILTER.

Construction checks Config and creates Resource before both SDK constructors; NewTracerProvider/NewMeterProvider return pointers, no errors. Metric construction registers the injected reader synchronously (private Reader.register); trace construction stores processor state without OnStart/OnEnd. Normal New return transfers sole ownership of both optional dependencies. No sharing/reuse after transfer. On validation failure nothing is registered/closed. Caller-supplied invalid/panicking dependencies follow SDK behavior, without recover. SDK global diagnostics are external callbacks; they are not a construction error/rollback API.

SDK findings, amended D45.4: explicit options override effective OTEL_* sampler/limits/exemplar/cardinality policy. Resource is the approved exception: both WithResource options merge resource.Environment(), so OTEL_RESOURCE_ATTRIBUTES/OTEL_SERVICE_NAME contribute keys, with present Config fields winning. SDK constructors also evaluate defaults before options and may report invalid settings through the global error handler. Meter config evaluates resource.Default internally; this module never requests/merges that default itself. Do not claim no internal SDK reads/caches/diagnostics. Future wrappers depend only on API TracerProvider/MeterProvider, never this module or the SDK (D45.8).

Shutdown is a single SDK-close attempt with the first caller's ctx. Both SDK providers are attempted and errors joined. Concurrent/subsequent callers wait for its cached result, or return their own context error/cause while waiting. Completed result wins for an already completed call; nil ctx always ErrNilContext. No own locks held over SDK callbacks; no goroutine abandons a noncooperative callback. Native trace Shutdown sets its closed bit before processors and may skip processor shutdown if ctx is already canceled, with no retry possible. Native meter shutdown uses a once-only reader fanout; repeated direct SDK calls can return ErrReaderShutdown. Our cache preserves the first result, including failure. Closed means SDK shutdown was invoked, not confirmed flush/resource disposal on error. Backend callbacks must not recursively wait on their owner's shutdown. Panics/Goexit propagate; abnormal close completion must wake waiters with an explicit incomplete-close error, not silently report success.

## Inventory and migration map

| Origin | Destination | Action | Test |
|---|---|---|---|
| pkg/telemetry/otel/entity.go OTELConfig/Provider | independent Config/Provider + API getters | adapt, no alias | defaults/zeros/interface consumer |
| provider.go NewProvider(ctx,cfg) Enabled branch | New(cfg,deps) | omit feature when unneeded; no Enabled flag | four signal combinations, no exporter |
| provider.go SamplingRate<=0 normalization | ParentBased ratio with explicit zero | deliberate semantic change | root/local/remote parent sampling |
| provider.go OTLP grpc exporters + headers/TLS | later transport modules | exclude | graph/package boundary |
| provider.go WithBatcher/PeriodicReader | caller-built injected processor/reader | exclude managed queues | registration/ownership/shutdown |
| provider.go Resource.New + default config | explicit NewWithAttributes semconv1.43 | adapt | OTEL_* canaries/resource equality |
| global.go ownership tokens + New default globals/propagator | assembler recipe only | exclude global registry | New slots unchanged, isolated recipe |
| provider.go Shutdown + sync.Once global release | cached one-attempt shutdown | adapt, no retry guarantee | concurrent/failure/context/gates |
| telemetry.go counters/histograms/cache/facade | native Tracer/Meter | no facade migration here | native reader/span integration |
| helpers.go/middleware.go | future API-only instrumentation wrappers | exclude | composed middleware API-only imports |
| provider/otel/otel.go Init/From/Close + engine adapter | legacy left unchanged | later engine migration | root regression |

Legacy constructs gRPC exporters, batch processor and periodic reader, normalizes sampling0 to1, installs globals/propagator unless SkipGlobalProviders, and tracks ownership through global token state. Telemetry facade caches instruments and wraps operations; providers integrate engine.RawConfig and lifecycle. Those production files/tests/consumers remain unchanged. Actual SDK code/doc checked: sdk trace/provider.go, span.go, span_limits.go and internal/attrnorm/truncate.go; sdk/metric provider.go/config.go/manual_reader.go/pipeline.go; resource/resource.go; semconv/v1.43.0 attributes.

## Increment evidence

C1: preflight, real SDK docs/code, executable native-limit proof, approved scope correction, inventory and closed contract recorded before production code.
C2: Config/defaults/errors and representation compile; deterministic validation and explicit-zero sampling tests pass.
C3: Native providers, four signal combinations, Resource, parent-based sampler and API getters pass with SDK capture and ManualReader.
C4: Native count/length behavior is tested across Start, SetAttributes, context access, events, links and RecordError; the approved span-only length scope is explicit. Resource environment merge has a separate both-signals test; sampler/span/event/link limits retain conflicting t.Setenv isolation tests.
C5: Transferred ownership, one-attempt shutdown, cached errors, concurrent waiters, canceled first close, noncooperative close, panic and Goexit tests pass; the globals recipe runs in an isolated subprocess.
C6: Unit/race, stress, four fuzz targets, standard goroutineleak profile and benchmarks implemented. Executed results follow below.
C7: Catalog/workspace registration and independent minimal/composed consumers pass offline; composed observer imports API trace and client only.
C8: README, MIGRATION, CHANGELOG, root references and candidate evidence included. No publication or version approval.

## Acceptance

E1–E5 executed with the scope/results below; E6 version proposal telemetry/otel/v0.1.0 awaits approval; E7 real-tag consumers are deferred. No commit, push, tags or release workflow.

## D45.4 discovery (resolved by the approved amendment below)

The effective-resource isolation test fails on installed SDK v1.47.0. Both sdk/trace/provider.go WithResource and sdk/metric/config.go WithResource call resource.Merge(resource.Environment(), explicitResource). OTEL_RESOURCE_ATTRIBUTES=injected=value therefore appears alongside the explicit service.name. The Config keys win on conflict; additional environment keys survive. Passing a Resource explicitly does NOT prevent this merge. Earlier C1 wording that explicit options isolate the resource was incorrect and is superseded by this finding.

The implementation was paused and the user then approved native SDK behavior: no facades, global-environment mutations or dependency upgrades. The formerly failing assumption was replaced by a dedicated Resource merge test; no effective Resource-isolation guarantee remains.

## D45.4 amendment approved by the user

The user approved native OTEL_RESOURCE_ATTRIBUTES/OTEL_SERVICE_NAME merging for both signals, with explicit Config fields taking priority. This supersedes the pending proposal above and the original resource-isolation wording. Extra environment keys survive; optional Config fields left empty do not override corresponding environment keys. Sampler and SpanLimits remain explicitly configured and isolated from OTEL_*; their t.Setenv regression is retained separately. No environment mutation, facade or SDK upgrade is introduced. TestResourceEnvironmentMergeConfigWinsBothSignals verifies Config precedence and extra keys in captured spans and collected metrics.

## Reproducible commands and executed evidence (E1–E7)

Executed 2026-11-13 on Go1.27.4 darwin/arm64 (Apple M1 Max) and Go1.27.4 linux/arm64 (local execution container, 4 CPUs/4 GiB, network disabled). No AMD64 validation is claimed. macOS root lint uses golangci-lint2.14.0. The host toolchain/cache were selected explicitly; no repository Go directive or existing dependency version changed. OTel API/trace/metric/sdk/sdk-metric remain exactly v1.47.0. Preparing the offline module cache used existing root-selected yaml/v3 v3.0.6 and x/sys v0.50.0, not an upgrade.

All commands run with GOTOOLCHAIN=local, GOPROXY=off and a prepared module cache; module/consumer commands additionally use GOWORK=off and GOFLAGS=-mod=readonly. Generic campaigns set their own offline environment. From the root:

```sh
scripts/modules.sh validate
scripts/modules.sh lint-lists
scripts/check-module.sh telemetry/otel deps
scripts/check-module.sh telemetry/otel consumers
scripts/check-module.sh telemetry/otel quick
scripts/check-module.sh telemetry/otel stress
TELEMETRYOTEL_FUZZ_TIME=3s scripts/check-module.sh telemetry/otel fuzz
TELEMETRYOTEL_BENCH_TIME=100ms scripts/check-module.sh telemetry/otel bench
make lint
make lint-arch lint-docs check-modules
make test
scripts/check-module.sh client consumers
scripts/check-module.sh client/logging consumers
scripts/check-module.sh logging consumers
```

All listed checks passed on macOS ARM64. Root make test includes legacy telemetry/provider and every catalog module; it needed local test listeners, not external services. Two lint findings in this module's tests were corrected (intentional nil-context contract test and fixed-selector self-execution of the globals recipe); final root lint and module lint report zero issues. Root go.mod/go.sum are unchanged. No Makefile/workflow/script modifications were needed for module registration.

`quick` executes go test -race -count=20 with default GOMAXPROCS, 1 and 4, then go vet. The complete final quick includes the explicit leak-profile test. macOS coverage96.4% is diagnostic only. Standard goroutineleak profile reports total0, including a separate 20-count run. Stress/count3 executes 240,000 spans/measurements across 300 owned provider lifecycles with concurrent use/close, plus 7,500 abandoned, properly closed empty providers. The warmed post-GC memory fixture allows8MiB for SDK/runtime caches; measured macOS heap pairs457,928→454,824,499,640→499,640,513,992→513,992 bytes. These fixture-specific observations are not a general memory bound.

Four 3-second fuzz campaigns on macOS: Config85,551; NativeAttributes74,000; Lifecycle71,832; ParentSampling71,979 executions. All passed. These bounded campaigns supplement deterministic sampling/limits/ownership tests; they are not a proof of arbitrary SDK correctness. Benchmark suite uses b.Loop (RunParallel/PB for concurrency), benchmem,100ms/count3: construction+shutdown, root/child spans, sampling on/off, attributes near limits, provider/instrument reuse, metric measurement, shared/independent concurrency. Native SDK reference is an equivalent span without attributes/processor; no exporter/I/O delay is included. Representative macOS sampled-span allocation528B/2allocs, unsampled144B/2allocs; observed costs vary and no SLA/zero-allocation promise is made.

Linux ARM64: the same module race/count20 matrix (default,1,4), vet, stress/count3, four fuzz targets at3s/parallel2, benchmarks100ms/count3 and both consumers' go run/go test-race passed. Linux fuzz executions Config79,934; NativeAttributes71,591; Lifecycle74,944; ParentSampling72,910. After adding the explicit leak-profile test, an additional 20-count leak run, full race suite and vet passed; additional benchmark cases were run separately. The source implementation did not change between these runs. Root multi-module regression/lint were run on macOS only; they are not claimed on Linux. The first Linux bind-mount attempt failed because the Colima daemon could not see the host /private/tmp archive; transferring prepared sources/toolchain/cache via stdin resolved it. The successful container used no network, sockets, collectors or services. Tar emitted harmless macOS extended-attribute warnings in the first successful archive transfer.

Linux reproduces module commands directly (same flags as quick/stress), with the official Go1.27.4 linux-arm64 archive and the selected module cache copied into a disposable local container. Source is a disposable snapshot; no toolchain/collector/container dependency is introduced in the module. Longer campaigns remain possible through the same generic commands; only the durations above count as executed.

Dependency evidence:24 selected modules including this main module,276 build/test dependency packages. Five direct OTel modules are declared in the catalog. Direct module tests use only stdlib and allowed OTel modules. SDK module metadata references its own test dependencies (including testify/goleak/YAML); those packages are not imported or compiled by this module's tests, and no testing dependency is added. No OTLP, gRPC, HTTP exporter or service SDK package is in the compiled graph. The generic guard verifies selected graph reachability, direct import ownership and exact local replaces. Consumers resolve independently offline: minimal excludes client; composed adds client. Its observer subpackage imports exactly context, client and API trace, proving the D45.8 boundary without a mock provider.

Gorelease: copied only go.mod/go.sum/provider.go into a temporary directory outside the working tree, then ran the externally installed gorelease binary there with `-base=none -version=v0.1.0`, GOWORK=off/GOPROXY=off. Result: “v0.1.0 is a valid semantic version for this release.” First version, no published base or historical API comparison. No tag or proxy was created. This checks a proposed version, not approval/publication.

Raw logs, graph/package lists and platform output are local ignored files in evidence/. They are not represented as versioned deliverables. This versioned summary retains commands, bounds, measured results and limitations; EVIDENCE.sha256 records the local evidence digests and SOURCES.sha256 the candidate files (excluding the hash files themselves). Key logs: quick-complete.log; stress/stress.log; fuzz/fuzz-Fuzz*.log; bench-final/benchmarks.log; leaks.log; root-lint-final.log; root-guards-final.log; root-test.log; consumers.log; related-consumers.log; gorelease.log; linux-arm64/{campaign,leaks-final,bench-delta}.log.

| Evidence | Result |
|---|---|
| E1 | Approved D45, span-only length clarification and revised D45.4 recorded in versioned README/RELEASE_NOTES and local contract. |
| E2 | Functional/race/resource checks and bounded stress/fuzz/bench passed on both ARM64 platforms as scoped above. Native SDK limitations remain explicit. |
| E3 | Real SDK span capture and ManualReader, shared Resource, native parenting/limits, partial-close failures and gates; no services. |
| E4 | Generic graph/replaces guards and both independent consumers passed offline; observer depends on API trace only, assembly uses factory/SDK. |
| E5 | External gorelease passed, first version with no base. No compatibility history fabricated. |
| E6 | Proposed independent v0.1.0 remains unapproved; catalog stays unreleased/-; distribution authorization deferred. |
| E7 | Consumers against a real module tag deferred until authorized publication. |

Remaining limits: no exporter/collector validation, managed processor, metric-cardinality policy, global ownership registry or automatic instrumentation. Native Resource environment merge and absent event/link/exception length limits are approved behavior. First shutdown error cannot be retried and does not guarantee flush; custom dependency callbacks can be noncooperative/panic. No root checks on Linux or any AMD64 execution are claimed. No commit/push/tags/index changes, release workflow execution or publication occurred.
