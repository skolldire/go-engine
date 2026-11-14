# client/otel

Typed middleware for logical I/O operations. Candidate, not published. Go1.27.4; OTel API v1.47.0, matching this repository. The module and its tests depend only on client, OTel API and stdlib, never on a provider SDK or telemetry/otel.

```go
cfg := clientotel.DefaultConfig()
cfg.InstrumentationName = "example.com/orders/client"
mw, err := clientotel.New[Order](cfg, clientotel.Dependencies{
    TracerProvider: applicationTracerProvider,
    MeterProvider: applicationMeterProvider,
})
// Check err, then pass mw to client.Do or client.Chain.
```

Import as `clientotel "github.com/skolldire/go-engine/client/otel"`. Supply native trace.TracerProvider and/or metric.MeterProvider. At least one is required. API no-op implementations are valid explicit dependencies. No globals are read/installed and no default provider is manufactured.

## Configuration and construction

Config.InstrumentationName is required; DefaultConfig leaves it empty. Literals are validated unchanged, without trimming/charset/length restrictions. Tags are mapstructure/json instrumentation_name. ConfigError{Field,Reason} wraps ErrInvalidConfig and never prints the invalid value.

Validation order: Config; typed-nil pointer TracerProvider; typed-nil pointer MeterProvider; both absent; IsFailure without TracerProvider. Nil pointer checks use reflection only in New; arbitrary invalid implementations are not universally detectable.

New captures Config by value, asks the supplied providers for scoped API instruments and creates one Float64Histogram. It starts no span/measurement and owns no resource, timer or goroutine. Provider methods are callbacks and can have their own effects. Instrument creation errors are wrapped with fixed context and preserve errors.Is/errors.As, without falsely marking them ErrInvalidConfig. A backend can include its own values in such errors: this module does not redact foreign causes. With a single histogram there is no multi-instrument registration/rollback protocol.

Providers/instruments are borrowed and never shut down, flushed or reconfigured. No global cache or per-label map exists. Reuse the middleware; shared backends and next must be safe for concurrent use. No callback is called under a module lock.

## Traces and status

One span per middleware invocation, named Component + "." + Operation, Kind Internal. This observes a logical call. If transport instrumentation such as otelhttp is inside it, the transport can create its Client child without duplicate logical Client spans in service maps.

Own attribute namespace (not an OTel semantic convention):

- go_engine.client.component and go_engine.client.operation at Start.
- go_engine.client.success after normal next return: err == nil.

success=false does not classify business/remote severity. No error text/type, payload, result, IDs, headers, baggage values, stack or panic content is added. No RecordError or automatic Ok status is issued. Ordinary caller attributes/status remain with the backend; the three own keys are used for this wrapper's observations.

Dependencies.IsFailure is optional. With tracing enabled and err != nil, it is called once after next and before End. True requests SetStatus(codes.Error, ""), with an empty description; false/nil leaves status untouched. It never affects metric dimensions. It is rejected without tracing. The caller chooses the error policy and callback concurrency safety; no universal classifier is provided.

Native status precedence is Ok > Error > Unset: a caller's Ok can prevent the later Error request from taking effect. The API cannot read status and this wrapper does not try. Status/error reporting is not a substitute for retry safety or reconciliation.

## One metric, in seconds

Float64Histogram: go_engine.client.operation.duration.
Unit: s.
Description: "Duration of normally completed logical client operations."
Attributes: the same component/operation/success keys.

One observation per normal return; use the histogram count for call counts. No separate calls counter. Views, buckets, aggregation, temporality, cardinality and exemplars are controlled by the MeterProvider. No claim of delivery/export/durability is made, including with a no-op provider.

Duration measures next (and inner middleware), stopping before classification/span finalization/metric callbacks. One time.Now clock with monotonic subtraction is used; there is no public clock option. Tracing-only does not measure a separate duration. Native span Start/End include some instrumentation overhead and need not match the histogram exactly. The histogram context retains the span context for backend correlation/exemplars, even after End. Unsampled spans do not suppress metrics.

## Context and results

With tracing, next receives the context returned by Tracer.Start, not necessarily the identical caller context. Native parenting, baggage, values, deadline, cancellation and cause are retained by a conforming provider. Metrics-only passes the exact original context. This module neither derives timeouts nor cancels/replaces the context with Background.

Invocation and Operation[T] pass through unchanged. Results and errors, including partial values, pointers, slices and typed causes, are returned intact and are never inspected via String/Error. A canceled context does not automatically prevent next; success returned after cancellation stays success. Do performs D35 validation; direct Chain callers assume those preconditions. Applying the middleware to next nil returns Handler nil.

## Abnormal completion and callback boundary

Normal next return adds success, applies optional IsFailure, closes the span once and records the histogram. Panic/Goexit closes the started span without inventing success or a measurement. IsFailure panic also closes the span and emits no completed histogram. No recover is used in production.

Span.End is called from a deferred closure, never directly as `defer span.End()`. SDK v1.47.0 End itself calls recover to record exception.message when directly deferred; calling through the closure avoids that path. The SDK-default harness proves span closure, unchanged panic identity, no exception event and no metric for panic/Goexit. No WithoutPanicRecording configuration is required.

The wrapper sends no panic value to the API. Arbitrary backends can independently record data, panic, Goexit or block. A backend callback can prevent return after completed I/O or replace a panic while unwinding; no isolation/recover/goroutine hides that behavior. An End call is not repeated if its callback panics. Slow callbacks block the caller.

## Composition and boundaries

Recommended: client/otel outside an operation wrapped by resilience. One logical span/histogram then includes admission, attempts and backoff. Placing the middleware inside attempts explicitly observes each attempt instead. No retries are discovered or inserted and the wrapper does not alter resilience.

Only decorate logical I/O; do not decorate individual transaction statements to imply atomicity. Component/Operation must be static, nonsecret and low-cardinality by caller responsibility. No runtime validator can prove those properties. The wrapper imposes no resource/sampling/limits policy: native Resource environment merge and D45's span-only length-limit scope remain those of the injected provider. This is not structural redaction.

## Independent checks

Use scripts/check-module.sh client/otel deps|consumers|quick|stress|fuzz|bench. Prefix CLIENTOTEL controls campaign duration/output. Tests use API-only local doubles/no-op providers. The nested integration module uses SDK capture/ManualReader and both direct SDK/factory providers. Consumers minimal and composed demonstrate the same boundary; only composed declares extra SDK dependencies through the generic consumer-deps data. See RELEASE_NOTES.md for exact executed platforms/results, hashes and pending release evidence.
