# Migration

The legacy pkg/telemetry/otel and provider/otel production code remains unchanged. No alias or equivalent bridge is provided.

Before: NewProvider(ctx,cfg) can construct OTLP gRPC exporters, normalize zero sampling to one, install globals and attach framework lifecycle. After: seed DefaultConfig, set ServiceName, construct your processor/reader explicitly, call New(cfg,deps), and explicitly Shutdown. Sampling zero is respected; ParentBased now honors parent decisions.

```go
cfg := otelprovider.DefaultConfig()
cfg.ServiceName = "orders"
cfg.SamplingRatio = 0
p, err := otelprovider.New(cfg, otelprovider.Dependencies{
    TraceProcessor: callerProcessor,
    MetricReader: callerReader,
})
// Handle err; use p.TracerProvider()/p.MeterProvider() as API dependencies.
// Stop instrumentation, then inspect p.Shutdown(shutdownCtx).
```

Endpoint/headers/TLS and exporters belong to the caller or later exporter modules. Queue/batch/flush settings belong to the caller's processor; this iteration has no managed processor. Ownership transfers only on successful New. A failed/canceled single-attempt shutdown is not retried and does not guarantee flush.

Globals are opt-in assembler calls to the OTel API, with no ownership/restore helper. Resource is constructed from Config but the installed SDK merges OTEL_RESOURCE_ATTRIBUTES/OTEL_SERVICE_NAME; Config's present keys win and extras remain. Sampler/limits are explicitly isolated. MaxAttributeBytes is replaced by MaxAttributeValueLength: SDK characters for span strings; no value-length guarantee for event/link/exception attributes. Counts are fixed as documented in README. There are no facade/middleware caches or automatic error/metric/span instrumentation here. Future wrappers take API providers only.
