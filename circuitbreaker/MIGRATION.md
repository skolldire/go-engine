# Migration

Legacy circuit_breaker and resilience stay unchanged, without new deprecations, aliases or equivalence wrappers.

Before:
```go
b := circuit_breaker.NewCircuitBreaker(circuit_breaker.Dependencies{
    Config: &circuit_breaker.Config{Name: "inventory"}, Log: log,
})
value, err := b.Execute(ctx, func() (any, error) { return read(ctx) })
```

After:
```go
cfg := circuitbreaker.DefaultConfig()
cfg.Name = "inventory"
b, err := circuitbreaker.New(cfg, circuitbreaker.Dependencies{Classify: classify})
if err != nil { return err }
value, err := circuitbreaker.Execute(ctx, b, read)
```

New is pure, defaults explicit and literals validated as supplied. Name has no implicit default. Typed operation receives the caller context. Partial values/errors remain intact; legacy discards values whenever an error exists. Pre-canceled calls never become admitted requests or remote failures.

Own State replaces gobreaker.State; StateHalfOpen spells half_open rather than the legacy presentation half-open. Stats is a generation snapshot, not a history. MaxRequests=100 becomes explicitly configured HalfOpenMax (default 1), bounding concurrent probes and required accumulated successes in that generation. Timeout becomes OpenTimeout; FailureRateThreshold becomes FailureRate. RequestThreshold and limits are uint64. Counts use completed Successes+Failures as denominator, excluding pending, ignored and abnormal calls; this deliberately changes opening behavior under concurrent load.

Caller classifies nonnil operation errors. Nonremote errors are Ignored, not Success. Caller cancellation usually is ignored; deadline expiry policy is application-specific. Panics and runtime.Goexit account as Panics and propagate without recover, rather than SDK panic-as-failure policy. HalfOpen with only Ignored/abnormal outcomes can persist indefinitely. Outstanding old-generation probes can continue physically; no global bulkhead or forced termination is promised.

Logging moves to future explicit wrappers. Future composition is retry outside breaker per attempt, not legacy resilience's breaker outside retry. Classify ErrOpen/ErrProbeLimit as Stop in retry. Unknown publication/commit outcome needs Reconcile, never a blind retry. Each application owns replay safety, timeout budget and SDK retry settings.

Proposed first version circuitbreaker/v0.1.0 is a local candidate. Manifests, publication and real-tag consumers are deferred; local replaces are not proof that go get resolves tags.
