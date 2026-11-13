# resilience

Explicit typed composition for I/O. D39/D39.1–D39.5 approved; D32 ordering conflict closed for this module. The single policy is:

**total budget → retry → rate admission → concurrency admission → circuitbreaker → attempt timeout → I/O**.

No SDKs, registry, observers, logger, background workers, recover or reimplementation of the tools. Dependencies: stdlib and client/retry/ratelimit/concurrency/circuitbreaker. Base modules remain independent. Even a minimal consumer resolves this declared module graph; it is not stdlib-only.

## API

```go
const OrderRetryThenBreaker = "retry_then_breaker"
type Config struct {
    Order string
    TotalTimeout time.Duration
    AttemptTimeout time.Duration
    AdmissionTimeout time.Duration
    Retry retry.Config
    Weight int64
    Tokens int
}
type Dependencies struct {
    Classify retry.Classifier
    Breaker *circuitbreaker.Breaker
    Limiter *concurrency.Limiter
    RateLimiter *ratelimit.Limiter
}
type ConfigError struct { Field, Reason string }
func DefaultConfig() Config
func (cfg Config) Validate() error
func Wrap[T any](operation client.Operation[T], cfg Config,
    deps Dependencies) (client.Operation[T], error)
```

DefaultConfig sets OrderRetryThenBreaker, retry.DefaultConfig and zero durations/Weight/Tokens. Zero retry repetitions means one execution; no presence flag or Config pointers. Literals are used unchanged: Config{} has an invalid nested Retry, not silent defaults. Validate checks nonempty Order if supplied, durations, Retry (including its causes), Retry.TotalTimeout==0, nonnegative Weight/Tokens. Wrap checks operation, Config.Validate, required Order with Breaker, required Classify for MaxRetries>0, Limiter/Weight, RateLimiter/Tokens. Only one supported Order; empty permitted without Breaker. Active admission tools require positive Weight/Tokens; absent tools require zero values. Nil Classify with MaxRetries=0 returns a single operation outcome without caller classification. Present Classify also sees the final error with MaxRetries=0, preserving Reconcile.

Wrap is pure: copies Config (including Retry), captures already-built resources/functions and constructs a closure. It does not execute I/O, query/change breaker state, acquire tokens/permits or create contexts/timers/goroutines. Caller owns dependency construction/lifecycle and callback concurrency safety. Each invocation owns local state; a wrapper is concurrently reusable. No Close or resource ownership transfer.

## Budgets and resource ownership

TotalTimeout>0 derives a context via WithTimeoutCause(ErrTotalTimeout) per logical call and cancels it on return. Zero preserves the caller context. Effective deadline is the nearest parent/local deadline. Budget includes admission, I/O, classification and backoff. Config.Retry.TotalTimeout must be zero: no duplicate total budget.

AdmissionTimeout>0 derives a fresh WithTimeoutCause(ErrAdmissionTimeout) context per attempt, jointly covering Wait then Acquire without restarting between them. The admission context is canceled before I/O and never passed to it. If no admission tools exist, this budget has no stage to govern and no context is created. A token consumed before a later admission failure or breaker rejection is not refunded. Acquire's release is deferred immediately after successful admission; it runs on errors, breaker rejections, success, panic and runtime.Goexit, before retry classification/backoff.

AttemptTimeout>0 derives WithTimeoutCause(ErrAttemptTimeout) only inside the breaker-admitted I/O callback (or immediately before I/O without breaker), and cancels it when the callback ends. No rate/concurrency waiting or backoff is charged to this budget. Shared permit/probe remains occupied through a noncooperative callback; no goroutine abandons it to simulate termination. Panic/Goexit propagate, and the base breaker accounts its own abnormal exits under D37.1. Retry-classifier abnormal exits occur after permit release.

Success stays exact value/nil even after cancellation. Partial values stay paired with their last attempt's error. A locally rejected attempt returns zero T; it does not resurrect a prior attempt's partial result/history. Total/attempt contexts are canceled on return: bodies, cursors and streams tied to them must be consumed inside the operation. For resources used afterward, set both budgets to zero and use caller-owned context/lifecycle.

## Errors and classification

Sentinels: ErrInvalidConfig, ErrInvalidCall, ErrAdmission, ErrAdmissionTimeout, ErrAttemptTimeout, ErrTotalTimeout. ConfigError unwraps ErrInvalidConfig; nested retry validation also retains its original cause. Validation names fields without printing values. Nil operation is ErrInvalidCall at construction; nil execution context returns zero T/ErrInvalidCall. Pre-cancellation preserves caller errors/causes and prevents I/O through retry.Do.

Wait/Acquire failures add ErrAdmission and a fixed stage name (ratelimit/concurrency), preserving their errors and the admission context's cause. In particular, Acquire alone only returns ctx.Err; this layer restores parent/custom/total/admission causes. These local failures never enter breaker classification. Oversized Weight (concurrency.ErrJobTooLarge) and Tokens (ratelimit.ErrInvalidCall) are application configuration errors detected at runtime: Wrap cannot inspect private capacities. Both preserve causes and Stop.

ErrAdmission, circuitbreaker.ErrOpen and ErrProbeLimit always Stop before the caller classifier; it never sees those categories. HalfOpen probe saturation therefore fails immediately rather than backing off/retrying locally. No universal SDK/context classifier or implicit attempt-timeout retry. Caller owns Stop/Retry/Reconcile, replay safety and SDK retry settings. Unknown publication/commit outcomes must reconcile, not blind retry.

I/O errors are enriched before breaker classification only when they represent the effective ended context. ErrAttemptTimeout is added only if err!=nil, the attempt context's cause is ErrAttemptTimeout and errors.Is(err, context.DeadlineExceeded). Success and independent business errors are untouched. If a parent wins, preserve its cause without local timeout attribution. Total/caller context-error causes are restored in this I/O boundary too: retry.Do returns original errors on Stop/exhaustion/nil classifier, so it cannot add a missing total cause on those paths. Retry normally preserves causes when canceling between attempts/backoff. At an exact timer/deadline boundary it can observe deadline expiry before the local context callback publishes its cause. On context-error paths, resilience settles an already-expired, locally-owned earliest budget via its Done channel, then restores the cause. The final total boundary also handles that case without attributing an earlier admission/attempt timeout to total; no new timer or goroutine is created. Classifier Stop/Reconcile/exhaustion precedence remains D36: an independent business/uncertain error is not rewritten merely because total time expired. All enrichment preserves errors.Is/As and partial values; otherwise error identity stays unchanged. Caller-provided causes may contain sensitive text; preserving them is not redaction.

## Accepted order costs

Rate-first avoids occupying concurrency while waiting for tokens, but spends tokens even if later stages reject. A concurrency queue after rate can group actual I/O starts; rate's guarantee applies at its own admission point, not final network instants. Concurrency-before-breaker avoids held HalfOpen probes during all admission queues, but can wait for tokens/permits merely to receive ErrOpen. There is no State precheck: it would not be atomic admission. Breaker sees each remote attempt, not the retry-aggregated result or local admission errors. Alternate orders are not implemented. D38.4 is resolved by approved D39.2; ratelimit's prior notes describe its historical pending decision, not a second current policy.

```go
cfg := resilience.DefaultConfig()
cfg.Retry.MaxRetries = 2
cfg.TotalTimeout = 5*time.Second
cfg.AttemptTimeout = time.Second
wrapped, err := resilience.Wrap(read, cfg,
    resilience.Dependencies{Classify: classify, Breaker: breaker})
if err != nil { return err }
value, err := wrapped(ctx)
```

Root targets: check-resilience and check-resilience-{quick,stress,fuzz,bench}. See MIGRATION and RELEASE_NOTES for executed evidence, dependency graph, release limitations and hash verification. No publication/stability claim.
