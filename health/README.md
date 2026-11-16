# health

Stdlib-only health registry, Go1.27.4, no require even in tests. Candidate, not published. No logger, driver, endpoint, background polling, environment/file access or dependency on another go-engine module.

```go
cfg := health.DefaultConfig()
r, err := health.New(cfg)
// Handle err.
err = r.Register("database", health.CheckerFunc(func(ctx context.Context) error {
    return db.PingContext(ctx) // db is borrowed and owned by the application.
}))
// Handle err; assemble all registrations before the first evaluation.
_, err = r.Ready(ctx) // ErrNotReady means a dependency is not up.
report, err := r.Check(ctx) // dependency failure is diagnostic, not this error.
```

## Configuration and registration

Config.Timeout defaults to5s, MaxParallel to4, MaxChecks to128. All must be positive, validated in declaration order. Literals are used unchanged; zero is invalid. Tags are mapstructure/json timeout/max_parallel/max_checks. ConfigError{Field,Reason} wraps ErrInvalidConfig without printing values. New copies Config, starts no worker/timer and does not allocate proportional to arbitrarily large configured limits.

Registry must be constructed with New and not copied afterward. Register validates names of1..128 bytes containing only ASCII A–Z/a–z/0–9/underscore/dot/hyphen. Nil Checker and typed-nil pointer/function implementations are ErrInvalidCall. No universal detection of other invalid implementations is promised. Names are unique (ErrDuplicateCheck), count is bounded (ErrTooManyChecks).

The first Ready or Check freezes the set, including an invocation with nil context. Later Register returns ErrRegistryFrozen, taking precedence over argument validation. Before freezing, concurrent registration is safe; valid registration order is lock acceptance order. Duplicate checking precedes the count limit. Live never freezes or executes checks: it always returns true on a constructed registry.

A checker is one named registration. Reusing a checker object under multiple names/registries does not deduplicate it; the application owns its concurrency safety and resources. The registry never closes the checker/client and has no Close/Shutdown.

## Readiness and diagnosis

Ready and Check run the same evaluation. Report contains Live, Ready and ordered Checks. No registered checks means Ready=true and a nonnil empty Checks slice. A successful check has Status "up", Reason "ok"; other results are "down" with Reason "failed", "timeout", "busy" or "panic".

Ready returns ErrNotReady if any result is not up. Check returns nil error for dependency failures, timeouts, busy entries and panics, allowing complete diagnosis. Both reject nil context with ErrNilContext. No extra error is returned merely because the caller canceled; that becomes a per-check timeout result. Empty-registry readiness remains true because no dependency failed.

All checks share an evaluation deadline bounded by the caller and Config.Timeout. Each admitted callback receives a child context of that bound; its deadline includes slot admission, rather than restarting after queuing. That child is canceled after callback termination. Values, parent deadline and cancellation cause reach the checker. Cancellation is cooperative: no callback is killed or replaced to pretend it finished.

Duration includes admission until completion/timeout; busy has zero execution duration. A result completed and published before timeout remains usable when another check times out. For normal callback completion, Err is the original error, preserving identity/errors.Is/errors.As. When the context times out before a result exists, Err is ctx.Err. No future error is available to report yet; late results are discarded rather than cached.

Report and Checks are fresh values/slices. Mutating them cannot change the registry or future reports. Err intentionally retains the callback's error identity; this is not a deep clone of arbitrary error graphs.

## Persistent slots and stuck callbacks

MaxParallel limits live checker executions across every evaluation of this registry, including callbacks still running after earlier reports timed out. Each named entry has at most one live execution. Observing that entry busy yields down/busy immediately, without spawning a replacement or joining/caching its old result.

A free entry waits in the caller's goroutine for a global slot until the context ends; no slot then yields down/timeout. Only admitted checker execution creates a module goroutine. No scheduler/polling goroutine, per-pending-check waiter goroutine or history exists. Entry liveness and the global slot count are synchronized; callbacks run outside locks. When a callback actually terminates, its slot is released and waiting evaluations can reconsider admission. There is no fairness guarantee or promise concurrent evaluations share one execution.

A noncooperative callback can remain alive indefinitely, but repeated polls cannot grow replacement workers. At most MaxParallel checker executions survive per registry; independent registries have independent budgets. The caller controls concurrent evaluations/instances. No global memory limit or ability to reclaim a forever-blocked foreign callback is promised. Implement checkers to honor their context.

## PanicError and private diagnostics

Only inside owned checker goroutines, panics are recovered so a probe cannot crash the process. Goexit is detected by completion/return flags and reported the same way. No recover is used elsewhere in production.

```go
type PanicError struct {
    Value  any
    Stack  []byte
    Goexit bool
}
```

A panic has the recovered Value and Goexit=false. runtime.Goexit has Value=nil and Goexit=true. The nested recovery/return flag also distinguishes legacy panic(nil) recovery from Goexit. Stack is captured with runtime.Stack inside the checker goroutine, current goroutine only, using a64KiB buffer; exceptionally deep stacks may be truncated. Error returns exactly ErrPanic.Error(), never formatting Value/Stack; Is recognizes ErrPanic. errors.As can retrieve *PanicError in both cases.

Health defines its own type without importing concurrency. It follows the same Value/Stack/fixed-error convention, with the explicitly approved Goexit field. Both abnormal cases release the entry and global slot and yield down/panic. No successful result is fabricated.

CheckResult.Err has json:"-". Standard json.Marshal(Report) excludes all errors, PanicError.Value and Stack, even if the underlying error implements json.Marshaler. No err.Error() is copied into another field. Other fields retain standard encoding/json names; Duration encodes as time.Duration's integer nanoseconds. Names/other application-supplied data are not secret-scanned. Router/operations must respect this boundary and not independently serialize diagnostic Err.

The application may deliberately log Err in a trusted channel. Value/Stack remain private diagnostic material, not redacted/safe for arbitrary logging. PanicError.Error is fixed; inspecting Value/Stack is an explicit caller choice. This module logs nothing.

Sentinels: ErrInvalidConfig, ErrInvalidCall, ErrNilContext, ErrDuplicateCheck, ErrTooManyChecks, ErrRegistryFrozen, ErrNotReady and ErrPanic.

## Adapter recipes, not core checkers

The application owns drivers and their lifecycles:

```go
// SQL: database/sql DB already built by the application.
r.Register("sql", health.CheckerFunc(db.PingContext))

// Redis: adapt the concrete client's result in application code.
r.Register("redis", health.CheckerFunc(func(ctx context.Context) error {
    return redisClient.Ping(ctx).Err()
}))

// HTTP: the application supplies client/url/status policy, and closes the body.
r.Register("remote", health.CheckerFunc(func(ctx context.Context) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, configuredURL, nil)
    if err != nil { return err }
    resp, err := httpClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 500 { return fmt.Errorf("remote status: %d", resp.StatusCode) }
    return nil
}))
```

These are illustrative caller snippets, not imports/dependencies or executed network tests of this module. Check Register errors in real assembly. HTTP /live,/ready,/deps belong to future router/operations, not this package.

## Verification

Use make check-module M=health MODE=deps|consumers|quick|stress|fuzz|bench; campaign prefix HEALTH. Fixtures use stdlib barriers/synctest/gates, including stuck-check goroutine bounds, both abnormal error shapes, JSON privacy and original-error identity. Minimal uses only health+stdlib; composed adds application-owned logging. See RELEASE_NOTES.md for actual commands, platforms, hashes and pending distribution evidence.
