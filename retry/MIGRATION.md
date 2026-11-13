# Migration

Legacy retry_backoff/resilience are unchanged and not deprecated by this candidate. No aliases or compatibility wrapper promise identical behavior.

Before:
```go
r := retry_backoff.NewRetryer(retry_backoff.Dependencies{
    RetryConfig: &retry_backoff.Config{MaxRetries: 3}, Logger: log,
})
err := r.Do(ctx, func() error { return send(ctx) })
```

After:
```go
cfg := retry.DefaultConfig()
cfg.MaxRetries = 3
_, err := retry.Do(ctx, cfg, classify, func(attempt context.Context) (struct{}, error) {
    return struct{}{}, send(attempt)
})
```

Caller owns classify and must justify replay safety. Unknown commit/publication outcome returns DecisionReconcile and requires application reconciliation; never blind retry. No universal SDK classifier. One layer owns retries and timeout budget; disable overlapping SDK retries explicitly where required.

Pointer normalization becomes DefaultConfig plus literal validation. Zero retries now means one initial attempt, rather than legacy zero normalized to three retries. No automatic defaults for invalid fields. Operation receives its effective context. A pre-canceled context prevents invocation. Additive jitter becomes full jitter (or explicitly none). Typed results retain partial values. Stop/exhaustion preserve original error identity; no final exhaustion wrapper or ErrExhausted. Reconcile and invalid decisions preserve original causes through errors.Is/As. Cancellation between attempts retains cancellation cause and last error.

Logging moves to explicit caller/client wrappers, not retry. TotalTimeout cancels its derived context at return: consume response bodies/cursors during the attempt; streaming uses TotalTimeout=0 and caller-owned context. A delay exceeding the remaining deadline returns the last error immediately, rather than waiting for expiry. The application still must validate idempotency and cooperative cancellation.
