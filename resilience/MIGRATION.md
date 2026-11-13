# Migration

Legacy resilience/BaseClient remain unchanged, without new deprecations, aliases or equivalence bridges.

Before:
```go
svc := resilience.NewResilienceService(resilience.Config{
    RetryConfig: oldRetryConfig, CircuitBreakerConfig: oldBreakerConfig,
}, log)
value, err := svc.Execute(ctx, func(ctx context.Context) (any, error) {
    return read(ctx)
})
```

After (new independent module):
```go
cfg := resilience.DefaultConfig()
cfg.Retry.MaxRetries = 2
cfg.TotalTimeout = 5*time.Second
cfg.AttemptTimeout = time.Second
wrapped, err := resilience.Wrap(read, cfg, resilience.Dependencies{
    Classify: classify, Breaker: breaker, // already built, caller-owned
})
if err != nil { return err }
value, err := wrapped(ctx)
```

Old breaker outside retry sees one aggregate outcome. New retry outside breaker sees each remote attempt; completed classified failure counts belong to circuitbreaker D37, not pending request totals or all business errors. Retries are explicitly enabled (default MaxRetries=0); uncertain outcomes require classifier Reconcile, never blind replay. Types replace any/SafeTypeAssert. Partial last-attempt values and errors.Is/As are preserved. Local rejection returns zero T rather than a prior attempt's payload.

Config owns Order, durations, Retry value, Weight/Tokens; Dependencies owns functions/prebuilt tools. No optional RetryConfig pointer or implicit zero defaults. Resource limits above supplied capacity cannot be inspected at Wrap: runtime ErrJobTooLarge/ratelimit.ErrInvalidCall retain causes and become ErrAdmission/Stop. No classifier callback sees ErrAdmission/ErrOpen/ErrProbeLimit. HalfOpen saturation is immediate failure, not a hidden retry.

Approved D39 closes D32: total → retry → rate → concurrency → breaker → attempt timeout → I/O. Rate guarantees apply at rate admission, not network start; later concurrency waits can group starts. No probe is held in admission queues. Tokens may be spent merely to receive ErrOpen; no refunds or racy State precheck. This deliberately replaces the old contract ordering.

Only resilience owns total budget; nested Retry.TotalTimeout must be zero. Admission budget is shared between rate/concurrency per attempt, canceled before I/O. Attempt timeout excludes queues/backoff and is classified inside breaker. Parent causes win when earlier. Success and independent business/uncertain errors are not rewritten just because a timer expired. Local timeout markers preserve original causes.

Context cancellation remains cooperative; noncooperative I/O retains its probe/permit until actual termination. Defer releases resources for panic and Goexit, without recover. Positive total/attempt budgets cancel returned context-tied streams/bodies/cursors: consume inside I/O or use caller-owned contexts with both budgets zero. No logger, metrics, lifecycle or automatic SDK retry configuration. Callers own resources and duplicate-budget avoidance.

Minimal still resolves all five base-module dependencies; activation is optional, the declared module graph is not. Local v0.0.0 replaces are development placeholders. Tags/manifests and real-tag consumers remain deferred to authorized publication.

## D43.1 correction (approved, not released)

Dependencies.Breaker/Limiter/RateLimiter are now minimal consumer-side interfaces. Existing concrete values still assign directly; foreign backends can implement them. Retry Config/Classifier remain concrete. Typed nil pointers are rejected at Wrap. Local breaker rejection joins ErrRejected and its original cause: replace equality comparisons against bare ErrOpen/ErrProbeLimit with errors.Is. Operation results/errors still travel through the callback. Nil release is a no-op; a real release runs once. The old concrete integration suite and fuzz corpus move to resilience/integration, not deleted.
