# ratelimit

Local token bucket, standard library only, including tests. D38 and D38.1–D38.4 approved. No background workers, maintenance timers, retries, SDK classifiers, distributed state or global registry. Do not copy a Limiter after New. Each instance owns an independent budget.

## API

```go
type Config struct { RatePerSecond float64; Burst int }
type ConfigError struct { Field, Reason string }
func (cfg Config) Validate() error
func New(cfg Config) (*Limiter, error)
func (limiter *Limiter) Wait(ctx context.Context, n int) error
func (limiter *Limiter) Allow(n int) bool
```

Both fields required; no DefaultConfig or implicit disabled mode. RatePerSecond must be finite and >0; Burst >0. Validate checks that order. Literals are used unchanged; New copies Config, opens no resources and starts no goroutines/timers. ConfigError unwraps ErrInvalidConfig. ErrInvalidCall identifies invalid Wait arguments without printing values. The application omits the limiter to disable limiting.

Bucket starts with exactly Burst integer tokens. Each admission atomically consumes n in [1,Burst]. No partial admission, reservation debt, Release or refund after application errors. Lazy monotonic-time replenishment saturates at Burst and discards time spent full; it cannot accumulate credit beyond capacity. This is a token bucket, not a fixed-window quota or a bound on simultaneous operations. concurrency.Limiter is a separate resource semaphore with release.

Allow never waits or starts a timer. It returns false for nil/uninitialized receiver, invalid n, insufficient tokens or any queued Wait. Invalid input has no state effects. Wait validates ctx, constructed limiter, n, context completion, then admission. Pre-canceled/expired callers do not enqueue or consume; their error preserves ctx.Err/context.Cause without ErrInvalidCall. Wait never derives/cancels/substitutes ctx. Injected cancellation causes remain caller-owned and can themselves contain sensitive text; preserving them is not redaction. An invalid zero value is not supported; Wait returns ErrInvalidCall and Allow false.

Wait pending order is acceptance under the mutex, not goroutine creation time. Strict FIFO: only the head may consume; smaller later requests and Allow never bypass it. Head-of-line blocking is intentional. Waiting calls own their queue nodes and channels. Only the head arms the instance's single reusable replenishment timer; other calls wait on their own head-notification channel or context. Head removal stops the timer and notifies only the next. Empty queue releases its timer/node references. No scheduler goroutine or per-call goroutine is created. Caller context timers are separate.

Cancellation observed under synchronization before consumption removes the waiter without consuming. If admission won, Wait returns nil and later cancellation does not undo tokens. Caller execution can race later cancellation and must cooperate. Wait does not run or cancel the caller's operation. Deadline is a real waiting bound, not an estimate: unlike retry D36.2, ratelimit does not reject predicted long waits because earlier FIFO cancellations may shorten them. A deadline observed at its actual instant prevents admission even if the context cancellation callback has not yet run; available cancellation causes are preserved.

## Exact arithmetic

Rate is the exact represented float64, not an exact decimal. Available tokens are integers. A monotonic anchor preserves the uncredited elapsed-time phase using integer numerator units; no floating token balance accumulates. Refill computes whole credit and retains the remainder in that anchor. Burst/n are never rounded through float64, including above 2^53. Integer arithmetic uses stdlib math/big with bounded operand sizes derived from float64 rate, Burst and elapsed nanoseconds; no call history is retained. Temporary integer buffers are reused under the instance mutex. Exact products are clipped only when they reach the missing capacity, never merely because float64 would lose precision.

For every interval [t0,t1], admissions are at most Burst+floor(rate*(t1-t0)), evaluated exactly. No over-admission tolerance. Arithmetic does not lose fractional time; waiting deadlines are rounded up by less than 1ns (the permitted bound is 1ns per observed replenishment). This numeric bound is not a scheduling or FIFO latency SLA: Go scheduling, earlier callers and head-of-line blocking can delay admission. Delays exceeding MaxInt64 nanoseconds are handled with cancelable bounded timer chunks, never overflowing or admitting early. Refill clips to Burst before converting unbounded products back to machine integers. No extra configuration limits are invented for extreme finite rates.

No callbacks, recover, env reads, observers, lifecycle, Stats or mutable configuration. Memory is proportional to the instance and current waiters, not completed calls. There is no MaxWaiters: caller controls goroutine count and upstream admission. Pending calls must complete or have their contexts canceled; dropping a Limiter reference does not cancel an active Wait. Multiple instances do not share remote quotas. SDK quota queries and throttling errors do not configure this limiter or enable retries automatically.

```go
cfg := ratelimit.Config{RatePerSecond: 25, Burst: 50}
limiter, err := ratelimit.New(cfg)
if err != nil { return err }
if err := limiter.Wait(ctx, 1); err != nil { return err }
return send(ctx)
```

## Composition boundary (D38.4)

The example uses client logical call → retry → ratelimit.Wait → circuitbreaker.Execute → I/O. Wait consumes retry's caller budget, occurs before breaker admission, and its cancellation never becomes a classified remote failure. Tokens can still be consumed when the following breaker rejects. ErrOpen/ErrProbeLimit are Stop for retry; uncertain publication/commit outcomes require Reconcile, not blind replay.

This is not the final resilience ordering. D32 conflict pending that module: its current contract places breaker before limiter, avoiding token consumption on ErrOpen but occupying a HalfOpen probe while waiting. Limiter-first avoids holding that probe but charges tokens for later breaker rejection. This iteration documents both costs and does not change resilience contracts or existing SDK policies.

Run make check-ratelimit and check-ratelimit-{quick,stress,fuzz,bench}. See RELEASE_NOTES.md for actual platforms/evidence, MIGRATION.md for scope. Publication/manifests and real-tag consumers remain separate tasks.
