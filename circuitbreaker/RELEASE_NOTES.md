# Circuitbreaker candidate

Initial branch feature/split, HEAD d87e888 (retry). `git status --porcelain` was empty. Go go1.27.4 darwin/arm64. No pre-existing work changed. Proposed version circuitbreaker/v0.1.0; no publication authorized.

## C1 — approved contract and inventory

D37 and D37.1–D37.5 approved by architect via user prompt. README.md records the contract before implementation. Stdlib-only, no gobreaker. Explicit caller classification; completed classified outcomes determine failure rate. Fixed lazy windows, pointer-identity generations, typed results, no recover; panic and runtime.Goexit account as Panics. Nil Classify is ConfigError/ErrInvalidConfig. Counter rollover follows D37.2.

| Origin | Destination | Action | Required evidence |
|---|---|---|---|
| pkg/utilities/circuit_breaker/entity.go Config/Dependencies | Config/DefaultConfig/Validate/Dependencies | adapt, no normalization or logger | configuration and validation matrix |
| service.go NewCircuitBreaker/Execute | New/Execute[T] | develop stdlib machine, typed/context-aware boundary | state, context, outcome, concurrency tests |
| gobreaker.State and Counts | own State/Counts | adapt, no SDK exposure | snapshots/invariants/allowlist |
| createReadyToTripFunc Requests denominator | completed Successes+Failures denominator | intentional change | pending/ignored do not dilute failures |
| MaxRequests=100/Timeout | HalfOpenMax=1/OpenTimeout | explicit new semantics | bounded probes/recovery tests |
| gobreaker generation uint64 | private generation identity | replace | late completions/rollover fuzz |
| automatic state/failure logging | future wrappers | exclude | no logger dependency |
| resilience breaker outside retry | consumer retry outside breaker | document future composition, retain legacy | composed consumer, legacy regressions |

Legacy production pkg/utilities/circuit_breaker uses github.com/sony/gobreaker v1.0.0 and logger.Service. Its Config pointer is copied; empty/nonpositive fields normalize to Name default-circuit-breaker, MaxRequests=100, Interval=60s, Timeout=30s, RequestThreshold=5, FailureRateThreshold=0.5. The factor test does not reject NaN. The wrapper exposes gobreaker.State and StateAsString, operation func()(any,error), and checks ctx.Err inside an already-admitted callback. On any error it discards the result, translating gobreaker rejection sentinels to local ones. The SDK's default success predicate counts every nonnil error as failure; it recovers panics, accounts failure and repanics. Its panic-recovery path does not finalize runtime.Goexit; the new defer/flag explicitly covers it. Failure threshold uses Requests (including pending) rather than completed classified outcomes. Interval resets closed counts lazily; Timeout governs open recovery; MaxRequests limits half-open requests/recovery. Callback state changes log with context.Background. New module deliberately changes all of these boundaries; no compatibility wrapper promises equivalence.

Direct production consumers: pkg/utilities/resilience/{entity,service}.go and http/pkg/rest/{entity,breaker}.go (transport wrapper, error alias); indirectly pkg/core/client and its adapters. Test consumers: pkg/core/client, http/pkg/rest, database/redis, aws cognito/s3/sqs. Legacy production remains intact. Observability, SDK-specific classification, retry budgets, HTTP resource ownership and composition belong to callers/future wrappers.

## Completed increments

| Step | Result | Evidence |
|---|---|---|
| C1 | Approved D37.1–D37.5; inventory and contract before code | above, README, local answers/public-contracts updated |
| C2 | Local errors/defaults/config and ordered validation | TestConfigAndValidation |
| C3 | Lazy monotonic fixed windows and pointer generations | TestTransitionsAndWindows, TestCounterRollover, model-based fuzz |
| C4 | Typed admission, probe bounds, independent instances | TestConcurrentProbeLimitsAndLateResults, TestIgnoredRecoveryAndNonCooperative |
| C5 | Caller classification, partial values, contexts, panic and Goexit without recover | TestOutcomesContextAndClassification, TestAbnormalTerminationAndProbeRelease, TestLateAbnormalAndAccumulatedRecovery |
| C6 | Race/stress/leaks/fuzz/benchmarks, legacy regression | platform logs, arithmetic rational oracle and transition model |
| C7 | Module integration, guards and two offline consumers | scripts/check-circuitbreaker-*.sh, minimal/composed examples |
| C8 | Candidate notes/migration/source and evidence hashes | this file, MIGRATION, CHANGELOG, SOURCE.sha256 and evidence manifests |

## Findings and implementation choices

Generation identity replaces wrapping numeric generation IDs. Finalization observes window expiry under the same lock and ignores retired results. Operations and classification execute outside locks; classification can query State/Stats without deadlock. No recover occurs in production: a completion flag plus defer accounts panic/runtime.Goexit, including abnormal classifier exits.

Counter arithmetic never wraps. Rollover on admission is tested with valid MaxUint64 Requests fixtures; a valid generation cannot naturally overflow an individual terminal counter while InFlight>0 because Requests bounds the sum. Defensive finalization rollover is tested with explicitly synthetic private fixtures. The triggering finalization is counted in the clean generation; HalfOpen failure can then immediately open as usual. Open rollover preserves its deadline.

Rounding uint64 counters to float64 can make MaxUint64-1 failures out of MaxUint64 completions appear to be 100% failures. The implementation compares the exact integer ratio with the configured represented float64 using bounded mantissa/product arithmetic (stdlib math/bits), avoiding integer overflow and allocations. Tests/fuzz compare to math/big.Rat as an independent oracle; binary float threshold semantics are documented.

Early fixture corrections: the denominator test initially used distinct error instances against an identity classifier, so those outcomes were correctly Ignored; the fixture now passes the intended original error. The retention fixture initially based later synthetic advances on real time while the previous iteration had advanced the expiry into the future, accidentally keeping the current generation alive. It now advances from the generation's actual expiry and checks only genuinely retired objects. Neither finding changed public contracts. No production leak or race was discovered in qualification.

No redundant comments or recover in production. The only legacy-package addition is a test-only benchmark; all legacy production and previously delivered modules are unchanged.

## Executed qualification

Go1.27.4 on macOS ARM64 and Linux ARM64. Both platforms passed:

- Race count=20 at default GOMAXPROCS, 1 and 4, and vet.
- Fifteen Test functions overall: three opt-in stress tests are skipped by quick and executed separately with race/count=3 per platform; other tests include the in-memory transport integration.
- 384,000 shared-instance concurrent executions across stress runs, mixing success, failure, ignored, operation panic and classifier panic.
- 600 recovery/reopen cycles, 60,000 Open rejections, 384,000 probe-limit rejections and 10,200 late completions without changing newer generations.
- 5,760 abandoned instances with 64KiB classifier payloads and 36,000 retired generations collected. Long-lived instance retained only its current generation. Post-GC heap stayed within the measured fixture allowance (4MiB, not a general user-memory limit); goroutineleak reports total 0.
- Four fuzz targets per platform, 30s each/two workers: **14,281,279 executions**, no failures. Config validation, exact-rate arithmetic, bounded transition sequences with pending/late admissions and virtual time, and near-limit rollover. Transition expectations come from a small separate event model; arithmetic expectations from big.Rat, not a copy of the private helper.
- Benchmarks, 500ms/count=3: direct, Closed success/failure/ignored, Open rejection, HalfOpen admission/rejection, window renewal, transitions, shared-instance and independent-instance concurrency. Allocations recorded; contention and generation allocation costs remain visible.

macOS additionally passed full make test, targeted legacy circuit_breaker/resilience race tests, lint across modules, architecture/dependency/docs guards, and regressions of retry/client/config/concurrency independent consumers. Linux ran the new module and its consumers only; no AMD64 claim.

Serial same-host comparison (Go1.27.4, Apple M1 Max, no race, 500ms/count=3, initial successful I/O stub, construction outside timing): median legacy **101.3ns/op**, typed Closed success **99.63ns/op**, both 0B/0alloc in these fixtures. Do not infer an SLA or a material performance advantage from this small difference. New semantics differ under failures/ignored/panics and are not claimed equivalent. Generation replacement allocates (96B in the window-renewal fixture); no blanket zero-allocation claim. Full tables and raw outputs are preserved.

Diagnostic statement coverage is 97.1%, not an acceptance substitute. Deterministic tests cover the approved normal/abnormal outcomes, thresholds, cancellation, exact deadlines, resource ownership, stale results, counter and probe invariants.

## E1–E7

| Evidence | Status | Proof / boundary |
|---|---|---|
| E1 contract/documentation | passed | approved D37.1–D37.5, API/defaults/errors/classification/windows/ownership/migration |
| E2 functional/resources | passed locally | unit/race/vet/stress/leak/fuzz/benchmark evidence on both ARM64 platforms |
| E3 integration | passed | in-module memory transport lifecycle, unknown/business outcomes, independent consumer composition; no external service integration applicable |
| E4 independence | passed locally | minimal only circuitbreaker+stdlib; composed uses real client and retry with local replaces, offline package/module/graph/replace allowlists |
| E5 gorelease | passed | external tool, source copy outside Git, -base=none -version=v0.1.0; first version, no historical comparison |
| E6 manifest/authorization | deferred | separate task after modularization; common-version guard still fails |
| E7 real-tag consumers | deferred | requires authorized publication and resolvable sibling tags; v0.0.0 replaces prove local independence only |

Additional make check-modules failed under its existing shared-version rule: v0.0.0 versus v0.30.0 and v0.22.0 already tagged. HEAD config/viper already requires config v0.0.0; baseline and exact failure output are in evidence. No manifest/version sync/release workflow edits. make all is not claimed to pass.

## Reproduction and limitations

[Execution record](evidence/EXECUTION.md) contains exact commands, environment, platform scope, toolchain/container and hash verification. Root make check-circuitbreaker and check-circuitbreaker-{quick,stress,fuzz,bench} reproduce independent consumers and qualification. CIRCUITBREAKER_RESULTS_DIR chooses output; default is a fresh temporary directory. CIRCUITBREAKER_FUZZ_TIME and CIRCUITBREAKER_BENCH_TIME override campaign durations. Skipped stress tests are not counted as executed campaigns.

Closed has no concurrency limit. HalfOpenMax is a current-generation limit, not a global connection/bulkhead guarantee. Noncooperative old operations can remain active after transitions. HalfOpen can remain indefinitely with only Ignored/abnormal results. No automatic timeout, cancellation, replay safety or classifier is provided. Callers own callback concurrency, classification, resource cleanup and retry budgets. Shared names do not share state. No distributed coordination or persistence.

Candidate circuitbreaker/v0.1.0 is functionally validated, not published or declared stable. E6/E7 remain explicit distribution work. No staging, commit, push or tags performed.

## D43.2 approved correction

Additive Run(ctx,fn) error shares a private execution core with Execute[T]; no second state machine. Existing tests/assertions remain unchanged. New equivalence tests cover normal/ignored/failure, Open/ProbeLimit, operation/classifier panic and Goexit, cancellation/causes and nil validation. Dependency remains stdlib. Final D43/D44 commands/results are in root RELEASING.md and release-evidence/d43-d44; distribution gates remain pending as before.

### Final D43/D44 validation

Go1.27.4 macOS ARM64 and Linux ARM64: deps, consumers, quick/race count20 at default/1/4 and vet pass. macOS make all, make test, both isolated harnesses and18 standalone modules pass. Circuitbreaker/resilience/real harness stress3 and100ms/count3 benchmarks pass on macOS; real resilience harness race20/stress1 also passes on Linux. Root manifests stay byte-identical after GOWORK=off tidy. Policy snapshots reject tagged coordinated-to-unreleased imports and Logrus declarations/graphs for their intended reasons. Scope/details, commands and hashes: root RELEASING.md, Evidencias D43/D44; local logs release-evidence/d43-d44. No versions/tracks/tags/commits/publication are approved or created.

Production Go files plus go.mod fingerprint (sorted root-relative path + NUL + bytes + NUL, concatenated, SHA256): 388bf85d1c128e7b3659d7fbcf63faf014fccf56e3c77b3f977281dbab13ba55. logging factory production code is unchanged by D44; this fingerprint records the correction snapshot, not published compatibility. Earlier source manifests remain historical snapshots.
