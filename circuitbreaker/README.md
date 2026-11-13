# circuitbreaker

Local typed circuit breaker. Standard library only, including tests. D37 and D37.1–D37.5 are approved. No workers, timers, queues, retries, logging, SDK classifiers or shared registry. Instances with equal names remain independent. Do not copy a Breaker after New. Closed does not limit concurrency; HalfOpen limits probes of its current generation, not all physical calls.

## API

```go
type State string
const (
    StateClosed State = "closed"
    StateOpen State = "open"
    StateHalfOpen State = "half_open"
)
type Config struct {
    Name string
    RequestThreshold uint64
    FailureRate float64
    Interval time.Duration
    OpenTimeout time.Duration
    HalfOpenMax uint64
}
type Dependencies struct { Classify func(error) bool }
type Counts struct {
    State State
    Requests, InFlight, Successes, Failures, Ignored, Panics uint64
}
func DefaultConfig() Config
func (cfg Config) Validate() error
func New(cfg Config, deps Dependencies) (*Breaker, error)
func Execute[T any](ctx context.Context, breaker *Breaker,
    operation func(context.Context) (T, error)) (T, error)
func (breaker *Breaker) State() State
func (breaker *Breaker) Stats() Counts
type ConfigError struct { Field, Reason string }
```

Defaults: Name empty (required), RequestThreshold=5, FailureRate=0.5, Interval=60s, OpenTimeout=30s, HalfOpenMax=1. The application sets Name. Literals are validated unchanged; no zero normalization. Name is nonempty without trimming/charset/length restrictions. Integer limits and durations are positive; FailureRate is finite and in (0,1]. Validate checks declaration order. New validates config then Classify; nil Classify returns ConfigError{Field:"Classify"}. New copies configuration/dependencies, opens no resources and starts no goroutines. No lifecycle or Close is needed.

ConfigError unwraps ErrInvalidConfig. Other sentinels: ErrInvalidCall, ErrOpen, ErrProbeLimit. Validation messages name fields/arguments without values. Caller errors are returned unchanged and may themselves contain sensitive values. State/Stats require a pointer constructed by New; nil receivers and manually constructed zero values are outside their contract.

Execute validates ctx, breaker, operation, context completion, then admission. Invalid arguments return zero T/ErrInvalidCall. A pre-canceled/expired context returns zero T and preserves ctx.Err/context.Cause without ErrInvalidCall or accounting. Cancellation observed during lock acquisition also prevents admission. Callback entry can race later cancellation: operations must cooperate.

Admitted operations receive exactly the caller's context. No new deadline/context is created or canceled. Success, partial value/error and typed causes are returned unchanged, even after cancellation. An uncooperative callback is never abandoned and keeps its admission until it exits (its generation can meanwhile be retired). All callbacks run outside locks and shared classifiers must be concurrency-safe. Querying State/Stats from a callback is permitted.

Success increments Successes. A nonnil operation error invokes Classify exactly once, even for a retired generation: true means Failure, false means Ignored. Ignored is not success and does not enter the failure-rate denominator. Rejections, pre-cancellation and operation panics are never classified. There are no automatic SDK/context heuristics.

## State, time and ownership

Closed starts at New with a fixed Interval window. At expiry, the next Execute, State, Stats or finalization starts a fresh window at the observation time. Time uses monotonic time.Time; no background maintenance. Threshold is applied to completed classified results: Successes+Failures >= RequestThreshold and Failures/(Successes+Failures) >= FailureRate. Pending requests, Ignored and Panics do not dilute this rate. The integer ratio is compared against the exact represented float64 threshold without first rounding uint64 counters to floats (important near 2^64 and FailureRate=1).

Opening starts OpenTimeout and clears current-generation counts. Open rejects immediately with ErrOpen. Once the timeout is observed, a new HalfOpen generation admits at most HalfOpenMax concurrent probes; excess calls return ErrProbeLimit without queuing. HalfOpen closes after HalfOpenMax accumulated Successes; the first classified failure immediately reopens and starts a new OpenTimeout. Ignored and Panics release capacity without proving recovery or clearing accumulated successes. HalfOpen can persist indefinitely if it only receives unclassified/abnormal outcomes. It has no automatic probe timeout.

Each transition/window replacement retires the generation. Its pending calls still return their own results and run classification when applicable, but never change new counts/state. No generation history is retained; private pointer identity prevents integer generation wraparound. Stats returns an immutable value snapshot with State from the same observation. Separate State/Stats queries can observe different generations.

Current-generation invariant: Requests=InFlight+Successes+Failures+Ignored+Panics. Rejections and pre-canceled calls are excluded. Before counter overflow, replace the generation with one of the same state; the triggering admission/finalization counts in the new generation and old calls become late. HalfOpen successes reset. An Open replacement preserves its deadline. Such arithmetic exhaustion is tested with private fixtures, not 2^64 actual calls. A valid generation cannot naturally overflow a terminal counter while it has InFlight capacity, since Requests bounds their sum; defensive finalization rollover is still defined and tested.

Panics and runtime.Goexit in operations or classifiers propagate unchanged. A completion flag and defer release/account the admission without recover. Panics counts both forms of abnormal termination, never remote failures or successes. Retired generations remain unaffected by late abnormal exits. All result/callback payload ownership stays with the caller.

## Caller classification

```go
classify := func(err error) bool {
    if errors.Is(err, context.Canceled) { return false } // caller canceled
    if errors.Is(err, validationErr) || errors.Is(err, notFound) ||
        errors.Is(err, conflict) { return false }
    if errors.Is(err, context.DeadlineExceeded) {
        return countRemoteSlowness // application policy: true or false
    }
    return errors.Is(err, remoteUnavailable)
}
cfg := circuitbreaker.DefaultConfig()
cfg.Name = "inventory"
breaker, err := circuitbreaker.New(cfg,
    circuitbreaker.Dependencies{Classify: classify})
```

A caller cancellation does not prove a remote failure. DeadlineExceeded may indicate a slow remote (count true), or caller budget exhaustion (count false); choose explicitly. Validation, not-found and conflict business errors usually do not indicate remote unavailability. These are examples, not built-in classifiers.

Compose externally: client logical call → retry → circuitbreaker per attempt → I/O. Retry must Stop on ErrOpen/ErrProbeLimit; they are local rejections, not remote attempts. Unknown write/commit outcome belongs to reconciliation policy, not blind retry. This module imports none of those tools.

See MIGRATION.md and RELEASE_NOTES.md. Root targets: check-circuitbreaker, check-circuitbreaker-{quick,stress,fuzz,bench}. Consumers run offline with GOWORK=off and explicit dependency allowlists. Candidate publication/manifests and real-tag verification remain separate tasks.
