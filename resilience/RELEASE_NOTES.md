# Resilience candidate

Initial feature/split, HEAD 5762d7f (ratelimit), git status --porcelain empty. Go go1.27.4 darwin/arm64. Proposed resilience/v0.1.0, no publication authorized. Existing work untouched.

## C1: approved D39 and closed D32

D39.1–D39.5 approved by architect via user prompt. Contract recorded before implementation in README; local public-contracts updated. Policy values Retry/Weight/Tokens move to Config, objects/functions remain in Dependencies. Retry always present, MaxRetries=0 means one call; Config literals validate unchanged. D32 ordering conflict closed: total → retry → ratelimit → concurrency → breaker → attempt timeout → I/O. No State precheck, alternate order or guarantees changed in base tools.

| Origin | Destination | Action | Required evidence |
|---|---|---|---|
| pkg/utilities/resilience/entity.go Config with pointer legacy configs | new Config/Dependencies | adapt; no presence pointer, no construction | validation/capture tests |
| service.go NewResilienceService | Wrap[T] | replace construction with pure composition | no stage touched at Wrap |
| legacy breaker outside retry | retry outside breaker per attempt | deliberate rupture | transport attempts and breaker counts |
| legacy retry normalization/default3 | explicit Retry value/default0 | adapt | one execution, classifier final error/Reconcile |
| logger.Service and gobreaker.State access | caller/base module resources, future observability | exclude | dependency guard and no observer imports |
| legacy any/nil error results | typed results and causes | develop | partial value/error identity/Is/As |
| BaseClient resilience middleware | explicit typed wrapper | document, do not migrate | legacy regressions |
| SDK/adaptor retry/backoff/timeout policies | application decisions | retain unchanged | no duplicated hidden budget, standalone |

Legacy service constructs retry_backoff and circuit_breaker even with nil config; those tools normalize defaults and enable retries (zero becomes 3). Breaker encloses all retries and sees only the final aggregate error, counts all nonnil errors and discards partial result on failure. Operation receives the caller ctx through a closure but no explicit total/admission/attempt budget exists. Service logs final failures and open-state warnings; underlying tools also log. Service stores shared tools/logger; mutable BaseClient Use/configuration remains legacy caller concern. No generic typed result or caller error classification.

Real consumers: pkg/core/client/{entity,resilience}.go creates/stores middleware/service; AWS ses/sqs/ssm/s3/cognito/sns and dynamo config types, messaging rabbitmq/grpc and database gormsql/memcached/mongodb/sqlc/redis configs reference legacy resilience. HTTP/rest also owns transport retry/breaker independently. Existing SDK retries can duplicate application budgets; no automatic migration or policy rewrite.

## Completed increments

| Step | Result | Evidence |
|---|---|---|
| C1 | Approved D39.1–D39.5 and closed D32, policy fields moved to Config | above, README and local public-contracts/answers |
| C2 | Pure typed Wrap, explicit validation and value capture | TestConfigurationAndValidation, TestPureCaptureAndCombinations (16 combinations) |
| C3 | Total/admission/attempt contexts and causes, nearest parent deadline | TestBudgetsCausesAndErrorPrecedence, TestAdmissionParentCauseAndContextOwnership, budget fuzz |
| C4 | Per-attempt rate/concurrency/breaker ownership | TestOrderAndPermitBeforeRetryClassification, TestAdmissionBeforeProbeAndAttemptClock |
| C5 | Local Stop precedence, oversized runtime config, partial/typed errors, panic/Goexit | TestAdmissionJointBudgetAndPermanentErrors, TestLocalStopsAndBreakerPerAttempt, TestAbnormalReleaseAndNonCooperation, TestAttemptEnrichmentBoundary |
| C6 | Race/stress/leaks/fuzz/benchmarks and legacy regressions | platform logs and retained fuzz seed |
| C7 | Framework-free consumers, measured six-library graph and guards | minimal/composed, module-graph.log, scripts/check-resilience-*.sh |
| C8 | Candidate docs/migration, source/evidence hashes | README/MIGRATION/CHANGELOG, SOURCE.sha256 and evidence manifests |

## Findings and corrections

The equal-deadline/backoff test found a real cause-propagation boundary: retry can return context.DeadlineExceeded from its actual-deadline check before WithTimeoutCause's callback publishes ErrTotalTimeout. Context-error boundaries now settle an expired locally-owned earliest budget before reading its cause; final total restoration excludes an earlier marked attempt/admission timeout. This adds no timer/goroutine and never waits for or rewrites a successful/independent business result. The base retry implementation and D36 precedence are unchanged. I/O context-error restoration is also necessary with nil/Stop classifiers and exhaustion, where retry deliberately returns the original operation error rather than adding causes.

FuzzSequences found an incomplete reference model: after a remote failure below RequestThreshold, a subsequent success can bring completed results to the threshold while failure rate remains >=0.5, so the breaker can open on that success. The abstract model was corrected; testdata/fuzz/FuzzSequences/0db7db71e7942ff1 is retained. No base-module production correction was needed. Final campaigns ran after this fix and the additional recovery/typed-error regressions.

No State precheck, recover, background execution, cloned resource, silent Retry default, duplicate retry budget or public testing hook was added. The only legacy-package code addition is a test-only baseline benchmark. Existing module production source/go.mod/go.sum and release workflow remain untouched.

## Executed qualification

Go1.27.4 on macOS ARM64 and Linux ARM64. Both passed race/count=20 at default GOMAXPROCS, 1 and 4, plus vet. Seventeen Test functions overall: three opt-in stress tests skip during quick and run separately with race/count=3 per platform.

Final campaigns across both platforms:

- 192,000 concurrent shared-wrapper calls and 230,400 remote attempts with real rate/concurrency/breaker tools. Five outcomes: immediate success, transient retries, business Stop, pre-cancellation and uncertain Reconcile. All permits released; breaker completed counts matched expectations.
- 1,200 timeout/panic/runtime.Goexit/success cycles with permit/probe cleanup.
- 2,880 abandoned wrappers with 64KiB payloads and derived contexts/timers reclaimed through weak-pointer/two-GC checks. Post-GC heap stayed inside the measured 4MiB fixture allowance, not a bound on arbitrary caller allocations. goroutineleak profiles total 0.
- Three fuzz targets/platform, 30s each/two workers: **9,630,372 executions**, no final failures. Config/dependency combinations, independent outcome/attempt/charge model and virtual-time budget precedence, with bounded sequences/waits. Reduced prior model failure retained as a regression seed.
- Deterministic typed-error preservation before breaker classification; AttemptTimeout only on an actual matching deadline; business/success/uncertainty precedence; shared admission budget; no probes/permits during preceding queues; permit release before classifier/backoff; runtime oversized Weight/Tokens; noncooperative operation retaining resources; streaming context cancellation; post-failure transport recovery.
- Benchmarks 500ms/count=3: direct, inactive Wrap, each tool, all tools, construction, each budget, each rejection stage, controlled retry, shared concurrency and legacy-equivalent initial success. Controlled retry includes real 1ns waits, not pure execution overhead.

macOS additionally passed make test across all modules, targeted legacy resilience/BaseClient race tests, lint, architecture/dependency/docs guards and independent consumers of all base modules. Linux ran resilience and its consumers only; no AMD64 attribution.

Serial same-host comparison (Go1.27.4, Apple M1 Max, no race, 500ms/count=3, construction outside timing, constant successful I/O): legacy median 104.7ns/op, typed boundary 128.3ns/op, both 0B/0alloc in these fixtures. Both include retry and breaker but no rate/concurrency; the typed path performs explicit validation/context handling and the failure paths intentionally differ. No performance advantage/SLA or blanket zero-allocation claim. Full tables retain costs of resources, contexts, errors and construction.

Diagnostic statement coverage: 96.0%, not an acceptance substitute. Tests exercise observable composition contracts rather than duplicating/accessing base-module internals.

## E1–E7

| Evidence | Status | Proof / boundary |
|---|---|---|
| E1 contract/documentation | passed | D39.1–D39.5 approved; D32 closed; Config ownership, order, causes/resources and migration documented |
| E2 functional/resources | passed locally | unit/race/vet/stress/leak/fuzz/benchmarks on both ARM64 platforms |
| E3 real composition | passed | memory transport with all real tools, recovery, uncertainty and abnormal cleanup; no external services applicable |
| E4 independence | passed locally | minimal/composed offline, explicit six-library graph, exact module/package/replace allowlists; no root/framework/SDK |
| E5 gorelease | pending distribution | attempted external tool, -base=none -version=v0.1.0; circuitbreaker@v0.0.0 cannot resolve without published dependency versions; exact failure preserved |
| E6 manifest/authorization | deferred | separate modularization publication task; common-version guard still fails |
| E7 real-tag consumers | deferred | requires authorized publication/resolvable base-module tags; local replaces prove development independence only |

Additional make check-modules failed under the existing common-version rule: v0.0.0 versus v0.30.0 and last root tag v0.22.0. Existing config/viper already requires config v0.0.0; baseline and failure output preserved. No manifest/version-sync/workflow changes. make all is not claimed to pass.

## Dependency and ordering decisions

Six libraries in the measured graph: resilience plus client, retry, ratelimit, concurrency, circuitbreaker. They are permitted composition dependencies, all stdlib-only underneath. No SDK/config/logger/telemetry/root packages. Minimal cannot claim a stdlib-only module graph simply because features are inactive. Replaces/placeholder versions are local development scaffolding, not publishable dependency versions; E5/E6/E7 must fix distribution later.

D38.4 conflict is closed by D39.2, not left for implementation choice. Whole policy OrderRetryThenBreaker means total→retry→rate→concurrency→breaker→attempt timeout→I/O. Accepted costs: tokens spent before later rejection, concurrency queues grouping downstream starts, waits before learning ErrOpen. No atomicity is claimed from a State query and no alternate order is implemented. Historical pending notes in earlier module evidence remain historical; the approved current decision is versioned here and in local normative contracts.

## Reproduction and limitations

[Execution record](evidence/EXECUTION.md) contains exact commands, platform scope, toolchain/container and hashes. Root make check-resilience and check-resilience-{quick,stress,fuzz,bench} reproduce consumers and qualification. RESILIENCE_RESULTS_DIR selects output; default is a new temporary directory. RESILIENCE_FUZZ_TIME and RESILIENCE_BENCH_TIME control campaign durations. Skipped stress cases never count as executed evidence.

Operations/classifiers must cooperate with context and be concurrency-safe if shared. No forced termination, replay safety or SDK policy detection. Caller owns tools/lifecycle and bounded pending call count. Positive total/attempt budgets cancel returned context-tied bodies/streams/cursors: consume inside or use caller context with both zero. Config literals are not defaulted. Local admission/open/probe failures Stop, never reach caller classifier, and may discard a prior remote attempt's partial result because the latest rejected attempt returns zero T.

Candidate resilience/v0.1.0 is functionally validated, not published or declared stable. E5–E7 remain explicit distribution tasks. No staging, commit, push or tags.

## D43.1 approved correction and preservation map

The module now imports/requires only client and retry. RateLimiter.Wait, Limiter.Acquire and Breaker.Run are consumer-side interfaces. Wrap rejects typed nil pointers with reflect only during construction. Release nil is no-op; nonnil release runs once. Local rejection before callback entry joins ErrRejected with the backend error and Stops before caller classification. Operation errors/partial values remain intact when Run forwards its callback error. A foreign Run must complete the callback synchronously, at most once, with a supplied/derived context and unchanged panic propagation. A new test explicitly demonstrates two callbacks as an observable contract violation, not silently corrected. Retry remains concrete so Config fully represents the retry policy.

New independent tests use only local foreign implementations; original real-tool tests move unchanged in intent to a nonpublished harness. No original test/fuzz/benchmark function is lost. Namespace/import adaptations and real-value Stats/Allow assertions are confined to the harness. Exactly the local-rejection assertions change from bare ErrOpen/ProbeLimit equality to errors.Is on both original error and new ErrRejected, including the fuzz model's expected signal. Original operation-error identity assertions remain strict.

The composed consumer's same rejection assertion is corrected for the mandated wrapper and gains compile-time interface assignments. Its tool construction is unchanged. Minimal's unused tool require/replace entries are removed and its informational graph description corrected; it now selects resilience/client/retry, not six library modules.

| Original function | New location |
|---|---|
| BenchmarkDirect | resilience/integration/benchmark_test.go |
| BenchmarkWrapExecution | resilience/integration/benchmark_test.go |
| BenchmarkConstruction | resilience/integration/benchmark_test.go |
| BenchmarkBudgets | resilience/integration/benchmark_test.go |
| BenchmarkRejections | resilience/integration/benchmark_test.go |
| BenchmarkRetry | resilience/integration/benchmark_test.go |
| BenchmarkConcurrent | resilience/integration/benchmark_test.go |
| BenchmarkBoundarySuccess | resilience/integration/benchmark_test.go |
| TestMemoryTransportRecovery | resilience/integration/integration_test.go |
| TestNoGoroutineLeaks | resilience/integration/qualification_test.go |
| TestStressSharedTools | resilience/integration/qualification_test.go |
| TestStressBudgetsAndAbnormalExit | resilience/integration/qualification_test.go |
| TestStressReleasedClosures | resilience/integration/qualification_test.go |
| FuzzConfig | resilience/integration/qualification_test.go |
| FuzzSequences | resilience/integration/qualification_test.go |
| FuzzBudgets | resilience/integration/qualification_test.go |
| TestConfigurationAndValidation | resilience/integration/resilience_test.go |
| TestPureCaptureAndCombinations | resilience/integration/resilience_test.go |
| TestContextValuesErrorsAndRetry | resilience/integration/resilience_test.go |
| TestOrderAndPermitBeforeRetryClassification | resilience/integration/resilience_test.go |
| TestAdmissionBeforeProbeAndAttemptClock | resilience/integration/resilience_test.go |
| TestAdmissionJointBudgetAndPermanentErrors | resilience/integration/resilience_test.go |
| TestBudgetsCausesAndErrorPrecedence | resilience/integration/resilience_test.go |
| TestAdmissionParentCauseAndContextOwnership | resilience/integration/resilience_test.go |
| TestLocalStopsAndBreakerPerAttempt | resilience/integration/resilience_test.go |
| TestAbnormalReleaseAndNonCooperation | resilience/integration/resilience_test.go |
| TestReusableConcurrentWrapper | resilience/integration/resilience_test.go |
| TestAttemptEnrichmentBoundary | resilience/integration/resilience_test.go |

Original fuzz corpus retained byte-for-byte: resilience/integration/testdata/fuzz/FuzzSequences/0db7db71e7942ff1.

Validation commands/platforms/hashes and the remaining distribution gates are recorded in root RELEASING.md, Evidencias D43/D44. No version/track approval, commit, push or tags.

### Final D43/D44 validation

Go1.27.4 macOS ARM64 and Linux ARM64: deps, consumers, quick/race count20 at default/1/4 and vet pass. macOS make all, make test, both isolated harnesses and18 standalone modules pass. Circuitbreaker/resilience/real harness stress3 and100ms/count3 benchmarks pass on macOS; real resilience harness race20/stress1 also passes on Linux. Root manifests stay byte-identical after GOWORK=off tidy. Policy snapshots reject tagged coordinated-to-unreleased imports and Logrus declarations/graphs for their intended reasons. Scope/details, commands and hashes: root RELEASING.md, Evidencias D43/D44; local logs release-evidence/d43-d44. No versions/tracks/tags/commits/publication are approved or created.

Production Go files plus go.mod fingerprint (sorted root-relative path + NUL + bytes + NUL, concatenated, SHA256): fd8446b4cc81c6fba218236a64ac8ed6d57a9f49517c6dee402fe03edc9b84f6. logging factory production code is unchanged by D44; this fingerprint records the correction snapshot, not published compatibility. Earlier source manifests remain historical snapshots.
