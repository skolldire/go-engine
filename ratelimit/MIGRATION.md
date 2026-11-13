# Adoption and boundaries

No legacy generic token bucket API was found; no rename, deprecation, compatibility alias or historical benchmark is invented. All SDK/adaptor rate/throttling/retry settings remain unchanged.

```go
limiter, err := ratelimit.New(ratelimit.Config{RatePerSecond: 20, Burst: 40})
if err != nil { return err }
if err := limiter.Wait(ctx, cost); err != nil { return err }
return transport(ctx)
```

Both fields required, no DefaultConfig or silent defaults. Burst is initial/capped credit, not a fixed-window quota. Wait admission consumes tokens permanently; an operation failure does not refund them. Allow=false is local non-admission, not a remote failure. Wait does not execute or promise that the operation will start; cancellation can occur afterward.

concurrency.Limiter bounds simultaneous resources and provides release. ratelimit bounds admissions over time and has no release. Combine explicitly without replacing one with the other. Each instance has its own budget; multiple instances/processes can exceed a remote account's aggregate quota. SES GetSendQuota and similar calls only report remote quota; the application explicitly constructs its limiter. SDK throttling errors do not automatically create local limits or retries.

Pending Wait calls are FIFO with head-of-line blocking; Allow cannot bypass them. Caller owns the maximum pending goroutine count: no MaxWaiters field or global memory bound. Deadline waits differ intentionally from retry D36.2: FIFO cancellations can shorten waits, so no predicted-deadline rejection. Only actual cancellation/expiry prevents admission. Tokens already consumed are not returned by subsequent cancellation.

Example composition: client logical call → retry → Wait → breaker → I/O. Rate-wait cancellation does not enter breaker classification or hold a HalfOpen probe. A later breaker rejection still costs a token; retry must Stop on ErrOpen/ErrProbeLimit. Unknown write outcomes require Reconcile, not blind replay. SDK retries and application retries must not duplicate budgets.

D32 pending resilience: its current contract puts breaker before limiter, avoiding token cost on ErrOpen but retaining probe capacity during waits. D38.4 approves limiter-first only for this example; no definitive resilience order is asserted or changed here.

Candidate ratelimit/v0.1.0 remains unpublished. Independent consumers use explicit local replaces; real-tag consumers and module manifests await authorized publication.
