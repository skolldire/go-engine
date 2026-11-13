# Ratelimit candidate

Initial branch feature/split, HEAD 1d2760f (circuitbreaker). git status --porcelain empty. Go go1.27.4 darwin/arm64. No previous work changed. Proposed version ratelimit/v0.1.0; no publication authorized.

## C1: approved contract and inventory

D38/D38.1–D38.4 approved by architect via user prompt. README records the contract before code: integer tokens/exact elapsed-time anchor, zero over-admission, strict FIFO, no bypass, only head arms a reusable timer, no predicted deadline rejection. No defaults. No new module dependencies. No recover or background goroutines.

| Origin | Destination | Action | Evidence required |
|---|---|---|---|
| local ratelimit public contract, no generic implementation found | ratelimit | develop | token accounting, Wait/Allow, precision/FIFO tests |
| concurrency/limiter.go Acquire/release | concurrency unchanged | exclude; distinct simultaneous resource limit | dependency guard, migration explanation |
| aws/pkg/clients/ses/service.go GetSendQuota, integration/aws/adapters/ses.go | SDK/application quota policy | retain; informational remote quota | no SDK imports or automatic config |
| aws/pkg/integration/aws/client.go throttling retry policy; adapters/errors.go SDK/text classification | SDK/caller policy | retain; no migration or universal classifier | legacy regressions, composed explicit classifier |
| aws/pkg/clients/cognito/helpers.go rate-limit error mapping | adapter/caller | retain, not admission | no SDK knowledge in module |
| http/pkg/rest retries/breaker, messaging SDK backoff and pkg/utilities/resilience | later explicit composition | retain, no extra hidden policy | composed standalone and existing consumers |

No existing generic token bucket API or direct golang.org/x/time/rate use was found in repository Go source. HTTP resolves golang.org/x/time v0.12.0 through resty/v2 v2.17.2; the cached SDK imports rate in its own tests and offers an optional Allow() bool RateLimiter interface. No repository call to SetRateLimiter configures that interface. This is not an existing go-engine admission implementation or a dependency to copy into the new module; versions are recorded in evidence/legacy-time-dependency.log. No legacy rate-limiter API or equivalent performance baseline is invented. concurrency.Limiter is weighted acquire/release, not rate enforcement. SES quotas are queries, not actual local admission. Existing AWS throttling/retry and HTTP/messaging policies remain intact; overlapping SDK retries/limits remain caller-owned.

## D32 pending resilience ordering

Approved example order: retry → ratelimit → circuitbreaker → operation. Current resilience contract says retry → breaker → limiter per attempt. Conflict deferred to resilience, not silently resolved here. Limiter first avoids occupying HalfOpen probes during waits but consumes tokens on later rejection. Breaker first avoids charging ErrOpen but keeps a probe occupied while waiting. Neither is declared the final resilience order.

## Completed increments

| Step | Result | Evidence |
|---|---|---|
| C1 | Approved D38.1–D38.4, inventory and contract before code | above; README; local answers/public-contracts |
| C2 | Required config, ordered validation, local errors, pure New | TestConfigAndValidation |
| C3 | Integer tokens/exact time-anchor phase, safe saturation and long-duration chunks | TestIntegerCreditAndFraction, TestExtremeRatesBurstAndLongDelay, rational oracle fuzz |
| C4 | Atomic Allow and immediate Wait, no invalid-input effects | configuration/burst/context tests |
| C5 | FIFO, no bypass, cancellation precedence, head-only reusable timer | TestFIFOHeadOfLineAndSingleTimer, TestQueueCancellation, TestContextAndDeadlineSemantics |
| C6 | Race/stress/leaks/fuzz/benchmarks and full regression | platform logs, interval envelope and independent rational state model |
| C7 | go.work/Makefile, stdlib-only guard, minimal/composed consumers | scripts/check-ratelimit-*.sh; examples/standalone/ratelimit |
| C8 | Candidate docs, migration, version proposal and hashes | README/MIGRATION/CHANGELOG, SOURCE.sha256 and evidence manifests |

## Findings and choices

D38.2 is implemented with integer tokens and a monotonic anchor carrying uncredited fractional elapsed time as an integer phase. Exact represented rate is converted once to numerator/denominator per nanosecond; no floating balance or float64 conversion of Burst/n. Refill clips only when exact whole credit reaches missing capacity. A product not representable as float64 is handled by bounded integer arithmetic, never turned into fabricated Burst tokens. This preserves the approved zero-over-admission requirement. Saturated time.Sub intervals are processed in bounded-duration segments; long waits use cancelable MaxInt64 timer chunks.

Timers: one pointer owned by the live queue, armed only by its head. It is stopped before handoff, reused by the next head, and released on drain. Middle/tail cancellation unlinks in constant time without waking the head or broadcasting. Tests count actual timer identities across uninterrupted queues and inspect head-notification channels under deterministic barriers; no public Stats, test callbacks or background scheduler were added.

An initial test fixture read the timer pointer without the mutex: race reported a fixture read against later head cleanup. The inspection now uses the same synchronization; production paths were already locked. No production race, numerical over-admission or retention defect was found in qualification.

Initial benchmark measurements showed numeric temporary allocations on each call. Reusing those integer buffers under the instance mutex removed common-path per-call allocations in the measured fixtures. API/precision stayed unchanged; full qualification on both platforms and root regression were repeated after that edit. No redundant production comments, recover, public test hooks or prior-module edits.

## Executed qualification

Go1.27.4 on macOS ARM64 and Linux ARM64. Both platforms passed race count=20 at default GOMAXPROCS, 1 and 4, plus vet. Four opt-in stress tests are skipped by quick and executed separately with race/count=3. Fourteen Test functions overall, including in-module memory transport integration and leak profile.

Final campaigns across both platforms:

- 192,000 mixed shared-instance Wait/Allow/cancel calls; weighted admissions checked against the exact integer-rate envelope.
- 96,000 additional weighted waits across 192 independent instances (32 per run).
- 120 uninterrupted FIFO queues, exactly one reusable timer identity per queue, 1,920 head notifications, 2,880 cancellations, no notification broadcast to canceled nonheads. At every observed handoff only the head owns replenishment; context timers are outside this count.
- 2,880 canceled waits with 64KiB context payloads, 2,880 queue nodes and 180 stopped long-delay timers reclaimed through weak-pointer checks after bubble exit/two GCs. Post-GC heap plateau stayed within the 4MiB fixture allowance, not a general limit on user memory. goroutineleak reports total 0.
- Five fuzz targets per platform, 30s each/two workers: **13,322,510 executions**, no failures. Config/weights, exact credit and upward delay rounding, independent rational accounting, FIFO cancellation sequences, and every pairwise interval of generated public Allow admission histories. math/big.Rat is the oracle; no over-admission tolerance. Time and sequence sizes bounded; no massive real sleeps.
- Duration/Burst extremes, >2^53 integer weights, minimal/maximal finite rates, >MaxInt64 waits and elapsed intervals, exact timer/deadline tie, canceled-head early handoff, no refund after operation failure.
- Benchmarks 500ms/count=3: Allow success/rejection, Wait immediate, invalid call, partial/saturated refill, prepared queued cancellation, shared-instance contention and independent instances. No intentional rate sleeping included in immediate-path fixtures. Queue cancellation includes goroutine/context/channel/timer setup and synchronized cancellation, not pure limiter overhead.

macOS additionally passed make test for every module, lint across modules, architecture/dependency/docs guards and independent consumer regressions of circuitbreaker/retry/client/config/concurrency. Linux ran ratelimit qualification and its two consumers only; no AMD64 attribution.

Diagnostic statement coverage is 98.3%, not acceptance proof. Observed macOS medians (Apple M1 Max, Go1.27.4, 500ms/count=3): Allow success 102.1ns/op and Wait immediate 103.5ns/op, both 0B/0alloc in those fixtures. Queued cancellation 3232ns/op includes its harness/resources. Costs of extreme arithmetic, new instances, queues and errors are not promised allocation-free. No SLA, blanket zero-allocation claim or invented legacy comparison.

## E1–E7

| Evidence | Status | Proof / boundary |
|---|---|---|
| E1 contract/documentation | passed | approved D38.1–D38.4; API, precision, FIFO, cancellation, resources, adoption; resilience conflict explicitly deferred |
| E2 quality/resources | passed locally | unit/race/vet/stress/leak/fuzz/benchmarks on both ARM64 platforms; exact rational and interval oracles |
| E3 integration | passed | local transport, weighted shared budget, failed-operation token consumption and wait cancellation before transport; no external services applicable |
| E4 independence | passed locally | minimal only ratelimit+stdlib; composed uses real client/retry/circuitbreaker; package/module/graph/replace allowlists, offline/GOWORK=off/-mod=readonly |
| E5 gorelease | passed | external tool, temporary source copy outside Git, -base=none -version=v0.1.0; first version without historical base |
| E6 manifest/authorization | deferred | separate publication task; existing shared-version guard fails |
| E7 real-tag consumers | deferred | requires authorized publication/resolvable sibling tags; v0.0.0 replaces prove local independence only |

Composed consumer verifies logical-call/wait/breaker/transport order, transient failures then success, ErrOpen/ErrProbeLimit Stop policy, uncertain result Reconcile without replay, tokens spent on breaker rejection, and cancellation during a blocked rate wait without a held HalfOpen probe, remote call or error classification.

Additional make check-modules failed under the existing common-version rule: v0.0.0 versus v0.30.0, last root tag v0.22.0. HEAD config/viper already requires config v0.0.0; baseline and exact output are preserved. No manifest/version sync/release workflow edits. make all is not claimed to pass.

## Reproduction and limits

[Execution record](evidence/EXECUTION.md) contains commands, actual platform scope, toolchain/container and hash verification. Root make check-ratelimit and check-ratelimit-{quick,stress,fuzz,bench} reproduce checks. RATELIMIT_RESULTS_DIR selects output; default is a new temporary directory. RATELIMIT_FUZZ_TIME and RATELIMIT_BENCH_TIME override durations. Skipped stress tests are not treated as executed campaigns.

Integer accounting preserves fractional time without loss. The <=1ns bound is arithmetic upward rounding, not scheduling/FIFO latency: runtime scheduling and preceding callers can delay execution. Caller bounds pending goroutines; no MaxWaiters/global memory promise. Each instance is local; multiple instances sum their budgets. No automatic SDK quota synchronization, retry policy, operation execution, resource release or distributed coordination.

Candidate ratelimit/v0.1.0 is functionally validated, not published or declared stable. D32 ordering is pending resilience, E6/E7 pending distribution. No staging, commit, push or tags.
