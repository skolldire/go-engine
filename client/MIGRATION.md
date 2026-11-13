# Migration to typed client

This is an experimental candidate. pkg/core/client and its consumers remain
unchanged; nothing is retired, aliased or automatically migrated here.

| Existing API/behavior | New responsibility |
|---|---|
| BaseClient embedding / BaseConfig | Own transport plus typed Do or a prebuilt chain |
| Execute returning any | Operation[T] returns the concrete T |
| Invocation.Client | Invocation.Component; new type, no alias |
| Chain(middleware...)(base) | Chain[T](terminal, middleware...) returns handler/error |
| Nil middleware ignored | ErrInvalidCall; remove absent wrappers before construction |
| Mutable Use | Build a new chain explicitly and own publication/synchronization |
| SetServiceName / SetLogging | Explicit fixed labels and optional caller-owned wrappers |
| Timeout default 10s or preserved caller deadline | Caller supplies context or explicit deadline wrapper |
| Logging / retry / breaker implied by configuration | Separate explicit wrappers; not included here |
| SafeTypeAssert[T] | No assertion required for typed result |

## Before

```go
raw, err := base.Execute(ctx, "Fetch", func(ctx context.Context) (any, error) {
    return transport.Fetch(ctx)
})
if err != nil { return Record{}, err }
return legacy.SafeTypeAssert[Record](raw)
```

## After

```go
return client.Do(ctx, client.Invocation{
    Component: "repository", Operation: "Fetch",
}, func(ctx context.Context) (Record, error) {
    return transport.Fetch(ctx)
})
```

The new call preserves the original context/result/error; it does not derive the
legacy timeout context. It also preserves a nonzero result returned with an error.
If the application needs a deadline, derive it explicitly and call its cancel.
No assumption that every middleware invokes next once is added.

For reuse, write a terminal and build once:

```go
terminal := func(ctx context.Context, _ client.Invocation,
    op client.Operation[Record]) (Record, error) { return op(ctx) }
handler, err := client.Chain(client.Handler[Record](terminal), first, second)
if err != nil { return err }
```

The caller of handler owns invocation preconditions. Only Do validates them.
No exported base handler/Validate/New/WithMiddleware is introduced. Resource and
concurrency ownership of wrappers stays with the application. Preserve causes,
context values and parent cancellation in wrappers. Do not introduce client into
config, concurrency or utilities, or apply per-statement retry wrappers inside Tx.
