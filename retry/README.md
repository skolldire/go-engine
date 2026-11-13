# retry

Independent typed retries for I/O. Standard library only, including tests. No background workers, automatic logging, SDK classifiers or persistence. One caller owns the retry budget. Unknown commit/publication outcomes must be classified Reconcile, never blindly retried.

## Public contract (D36 approved)

```go
type Decision string
const (
    DecisionStop Decision = "stop"
    DecisionRetry Decision = "retry"
    DecisionReconcile Decision = "reconcile"
    JitterFull = "full"
    JitterNone = "none"
)
type Config struct {
    MaxRetries int
    InitialBackoff time.Duration
    MaxBackoff time.Duration
    TotalTimeout time.Duration
    Multiplier float64
    Jitter string
}
type Classifier func(error) Decision
func DefaultConfig() Config
func (cfg Config) Validate() error
func Do[T any](ctx context.Context, cfg Config, classify Classifier,
    operation func(context.Context) (T, error)) (T, error)
type ConfigError struct { Field, Reason string }
```

Defaults: MaxRetries=0, InitialBackoff=100ms, MaxBackoff=10s, TotalTimeout=0, Multiplier=2, Jitter="full". Literals are used unchanged. Validation requires nonnegative retries/timeout, positive initial backoff, max>=initial, finite multiplier>=1, full/none jitter. ConfigError unwraps ErrInvalidConfig; messages name fields without values. Other sentinels: ErrInvalidCall, ErrInvalidDecision, ErrReconciliationRequired. External operation errors remain caller-owned and may contain sensitive data.

Do validates ctx, config, operation, classifier (required if retries>0), then checks cancellation. Invalid calls return zero T. Pre-canceled calls return zero T and preserve ctx.Err/context.Cause, without ErrInvalidCall. No initial delay. MaxRetries counts repetitions, not total attempts. A nonnil classifier receives every error, including the last, even with MaxRetries=0; never success. With nil classifier and zero retries, error passes through.

Success returns the exact value/nil even after context expiry. Stop and exhausted Retry return the exact last value/error. Reconcile joins ErrReconciliationRequired with the original cause; unknown decisions join ErrInvalidDecision with the original error, without printing the decision. errors.Is/As preserve causes. There is no ErrExhausted. Panics propagate. Config and caller context are never mutated.

Before waits and subsequent attempts, cancellation returns the last value and an error retaining ctx.Err, context.Cause, and the last operation error. Success, Stop, Reconcile and invalid-decision outcomes have precedence over cancellation. Callback entry can race cancellation; operations must cooperate. No goroutine abandons a noncooperative callback.

TotalTimeout=0 passes the original context unchanged. A positive value derives/cancels a context on return; the earlier parent deadline wins. Budget covers callbacks, classification and waits. If a computed delay ends strictly after that deadline, return the last value/error immediately without sleeping or another attempt. An equal endpoint is allowed, then cancellation is checked again. With TotalTimeout>0 consume HTTP bodies/cursors/streams inside the attempt: returned resources tied to that context are canceled on return. For streaming use TotalTimeout=0 and a caller-owned deadline/context.

Before retry k (first k=0), cap=min(max, initial*multiplier^k), truncated to whole nanoseconds with saturation. None waits cap; full selects a uniform integer in [0,cap], including zero and MaxInt64 safely. Waits are cancelable; timers are stopped. No history proportional to retry count. A very large MaxRetries with no timeout and an always-Retry classifier is valid and has no practical time bound. Concurrent calls are independent; callbacks/classifiers must themselves be concurrency-safe when shared.

```go
cfg := retry.DefaultConfig()
cfg.MaxRetries = 2
value, err := retry.Do(ctx, cfg, func(err error) retry.Decision {
    if errors.Is(err, outcomeUnknown) { return retry.DecisionReconcile }
    if errors.Is(err, transient) { return retry.DecisionRetry }
    return retry.DecisionStop
}, operation)
```

Run `make check-retry-quick`, `check-retry-stress`, `check-retry-fuzz`, `check-retry-bench` and `check-retry` from the repository root. See RELEASE_NOTES.md for executed evidence; example consumers run offline with GOWORK=off, inside the dependency boundary declared in modules.tsv.
