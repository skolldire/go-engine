# Retry candidate

Initial state: branch feature/split; `git status --porcelain` empty. Go go1.27.4 darwin/arm64. No pre-existing work was changed. Proposed version: retry/v0.1.0. No publication authorized.

## C1: inventory and approved contract

D36 and D36.1–D36.4 are approved by the architect (user-supplied prompt). The complete public contract is documented in README.md before implementation. Literal configuration is validated unchanged. Retry is disabled by default. Classification belongs to the caller, including on the final error. Stop/exhaustion preserve original error identity; reconcile signals control and never executes reconciliation. Success wins over cancellation; cancellation before entry preserves cause, and cancellation between attempts also preserves the last operation error. An interval beyond the remaining deadline returns the last error immediately. Derived TotalTimeout contexts are canceled on return, including success.

| Origin | Destination | Action | Required evidence |
|---|---|---|---|
| retry_backoff/entity.go Config, NewRetryer defaults | retry.Config/DefaultConfig/Validate | adapt; no zero normalization | configuration matrix |
| retry_backoff/service.go Do | retry.Do[T] | develop typed/context-aware classification | counts, identity, context, panic tests |
| calculateWaitTime | private capped calculation/full jitter | adapt; no additive jitter or overflowing multiplication | arithmetic extremes, fuzz |
| logger.Service dependency and automatic logs | later client wrappers | exclude | stdlib guard |
| retry_backoff final wrapping | original error on stop/exhaustion | deliberate change | identity tests |
| resilience/service.go breaker outside retry | later resilience composition | retain legacy, no migration | legacy regressions |

Legacy: MaxRetries<=0 becomes 3; initial/max/backoff nonpositive become defaults; negative jitter becomes 0.2 but zero remains zero. Config is copied. Operation is func() error, runs once even with pre-canceled ctx. Context observed after operation returns its error (possibly nil); wait cancellation returns ctx.Err. time.After waits use exponential power converted to duration before multiplication, additive random jitter, then cap: overflow and fractional-factor truncation risks. Exhaustion wraps and logs error; intermediate failures log automatically. New behavior does none of this implicit normalization/logging.

Direct production consumer: pkg/utilities/resilience/{entity,service}.go, itself used by legacy BaseClient/adapters. Test consumers include pkg/core/client, database/redis, aws cognito/s3/sqs. Kafka/Cognito SDK retry-backoff configuration is SDK-owned; no automatic composition or duplicate retry budgets are introduced. No legacy production code is changed.

## Completed increments

| Step | Result | Evidence |
|---|---|---|
| C1 | Legacy inventory and approved D36.1–D36.4, contracts written before code | above; README.md; local answers/public-contracts updated |
| C2 | Config/errors/defaults/ordered validation | TestConfig, TestValidationOrder |
| C3 | Saturating cap, inclusive full jitter without MaxInt64+1 signed overflow | TestArithmeticExtremes, FuzzBackoff, recorded stdlib RNG API |
| C4 | Typed results; last-error classification; original identity, reconcile, invalid decisions, propagated panics | TestOutcomes, TestNonCooperativeAndPanics |
| C5 | Cooperative context/budget/timer ownership and approved precedence | TestContextAndPrecedence, TestBudgetAndContextOwnership, TestWaitCancellationAndExactBackoff, TestParentDeadlineCause |
| C6 | Unit/race/stress/fuzz/leaks/benchmarks and legacy regressions | evidence/darwin-arm64 and evidence/linux-arm64 |
| C7 | Stdlib-only guard, go.work/Makefile, offline minimal/composed consumers | scripts/check-retry-*.sh, examples/standalone/retry; composed uses real client |
| C8 | Candidate v0.1.0 documentation, migration, evidence/hashes | README/MIGRATION/CHANGELOG, SOURCE.sha256, evidence manifests |

## Findings and corrections

A deterministic equal-deadline test found a scheduling window: a timer could finish at the deadline before the context's cancellation goroutine updated Err(). Checking the actual deadline as well as ctx.Err() now prevents another attempt in that window. No public contract changed.

The retention fixture initially checked payloads after only one GC and inside a live synctest bubble. Canceled timer/context references can need a second GC, and synctest retains completed goroutine state until bubble teardown. The final test checks weak pointers after bubble exit and two GCs. This corrects the fixture, not a claimed production leak. All 1,280 canceled waits per run release their payloads afterward. Separate 960-object post-GC plateau tests run outside synctest, with a measured 4MiB fixture allowance; this is not a limit on arbitrary user allocations.

Only required nolint rationale remains in production code: Uint64N's [0,n) guarantee bounds the unsigned-to-duration conversion. No redundant narrative comments were added. Legacy production source and all previously delivered modules remain untouched; the only legacy-package addition is a test-only comparison benchmark.

## Validation results

Go 1.27.4; macOS ARM64 and Linux ARM64. Each platform passed race count=20 at default GOMAXPROCS, 1 and 4, plus vet. The final 13 Test functions include three opt-in stress tests; ordinary qualification skips those and stress executes them separately, three times/platform.

- 192,000 concurrent policy calls across final stress runs, mixing success after transient failures, stop, reconcile and cancellation, including full-jitter concurrency.
- 7,680 canceled, actually blocked hour-long virtual waits, with timers/contexts/payloads released after bubble exit.
- 5,760 canceled hour-deadline contexts carrying 64KiB payloads; weak pointers cleared and post-GC heap plateau stayed within the fixture allowance. `goroutineleak` profiles report total 0.
- Three fuzz targets per platform, 30s each/two workers: **14,293,415 executions**, no failures. Configuration, arithmetic/backoff and bounded decision-sequence invariants; no massive real waits or unbounded fuzz loops.
- Benchmarks 500ms/count=3, allocations recorded. Real 1ns timer waits in retry-success/exhaustion fixtures are part of their measurements and are not reported as pure execution overhead.
- Diagnostic statement coverage: 97.3%; not an acceptance substitute. Saturation, cancellation, ownership and error invariants have explicit behavioral tests.
- Full macOS `make test`, targeted legacy retry_backoff/resilience race tests, lint across modules, architecture/dependency/docs guards and existing client/config/concurrency consumers passed. Linux ran retry and its consumers only. No AMD64 execution claimed.

Serial same-host comparison (`evidence/darwin-arm64/comparison.log`, Go1.27.4, Apple M1 Max, no race, 500ms/count=3): median legacy initial-success call **2.601ns/op**, typed Do **10.95ns/op**, both 0B/0alloc in these fixtures. The new call performs explicit validation and pre-entry cancellation/deadline checks that legacy does not; default config creation/construction is outside timing for both. These are measurements, not a zero-allocation guarantee, performance SLA or prediction of application latency. Full retry/backoff/jitter/concurrent benchmark results are in each platform directory.

## Evidence E1–E7

| Evidence | Status | Proof / boundary |
|---|---|---|
| E1 contracts/documentation | passed | D36 approved; README ownership/error/default/precedence contract; migration/inventory |
| E2 quality/resources | passed locally | unit/race/stress/fuzz/vet/leak/benchmark logs on both ARM64 platforms |
| E3 composition | passed | in-memory transient failure then success; uncertain outcome reconciles without repetition; logical-call/attempt ordering through client; no external services applicable |
| E4 independent consumers | passed locally | minimal: only retry+stdlib; composed: retry+client+stdlib; module/package/graph/replace allowlists, GOWORK=off/GOPROXY=off/-mod=readonly |
| E5 gorelease | passed | external tool, temporary source copy outside Git, -base=none -version=v0.1.0; first version, no historical comparison |
| E6 manifest/authorization | deferred | separate modularization task; existing shared-version check fails as recorded below |
| E7 real-tag consumers | deferred | requires authorized publication and resolvable retry/client tags; local v0.0.0 placeholders do not prove go get |

Additional `make check-modules` failed, not waived as passed: existing shared-version check reports v0.0.0 versus v0.30.0 and last tag v0.22.0. HEAD config/viper already requires config v0.0.0. Exact output is evidence/darwin-arm64/check-modules-deferred.log. No sync-version script, manifest or release workflow was changed. `make all` is therefore not claimed to pass.

## Reproduction and limitations

See [execution record](evidence/EXECUTION.md) for exact commands, platforms, toolchain, container and interpretation. Run make check-retry-{quick,stress,fuzz,bench} and make check-retry from root. The opt-in stress suite must be run explicitly; skipped tests are not counted as a passing stress campaign. Fuzz/benchmark defaults can be overridden with RETRY_FUZZ_TIME/RETRY_BENCH_TIME. RETRY_RESULTS_DIR chooses evidence output; default is a temporary directory, preventing accidental overwriting of checked-in evidence.

No callback can be forcibly canceled. Caller classifiers own replay safety, retry/reconcile policy, resource cleanup inside attempts and concurrency safety. High MaxRetries with no TotalTimeout can run for a very long time. No per-attempt timeout, SDK-specific retryability, observability or persistent reconciliation is supplied. TotalTimeout contexts cannot own returned streaming resources beyond Do. No service-based integration is needed for this stdlib-only execution primitive; its independent, in-memory I/O composition is exercised instead.

Candidate is functionally validated, not published or declared stable. Proposed tag retry/v0.1.0 needs later authorization. No index changes, commit, push or tags were performed.
