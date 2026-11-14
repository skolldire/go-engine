# Migration from legacy client instrumentation

Legacy pkg/core/client.WithTracing/WithMetrics and all production consumers remain unchanged. No alias, bridge or deprecation is introduced.

Before (legacy):

```go
base.Use(client.WithMetrics(recorder), client.WithTracing(facade))
result, err := base.Execute(ctx, "read", func(ctx context.Context) (any, error) {
    return transport.Read(ctx)
})
```

After (new modules):

```go
mw, err := clientotel.New[Response](clientotel.Config{
    InstrumentationName: "example.com/orders/client",
}, clientotel.Dependencies{
    TracerProvider: applicationTracerProvider,
    MeterProvider: applicationMeterProvider,
    // IsFailure is optional and explicitly owned by the caller.
})
// Handle construction error.
result, err := client.Do(ctx, client.Invocation{
    Component: "storage", Operation: "read",
}, transport.Read, mw)
```

Recorder/Tracer facades become native API provider dependencies; the wrapper does not construct or close providers. Invocation.Client becomes Component. Typed T replaces any/SafeTypeAssert. Span naming remains Component.Operation but Kind is Internal for a logical call, allowing transport Client children. Own go_engine.client.* attributes replace facade-specific dimensions.

The old telemetry facade can RecordError and use err.Error in status; this wrapper never does. Default status is untouched; optional IsFailure requests Error with an empty description and respects native Ok>Error>Unset precedence. success=false means only that next returned an error. No error/result text is inspected. One seconds histogram includes normal call counts, with no duplicate counter. Panic/Goexit end spans through a closure without fabricating completion metrics or recording panic contents via the SDK's direct-defer path.

Explicit placement outside resilience observes one logical call including retries; inside attempts observes attempts. No lifecycle/default timeout/global provider or mutable Use policy is migrated. Resource/environment/sampling/limits remain provider decisions. This does not promise equivalent legacy telemetry streams, error classification or delivery guarantees.
