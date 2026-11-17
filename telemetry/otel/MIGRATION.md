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

## D56.7 Instrumentacion HTTP de servidor

`pkg/telemetry/otel.NewMiddleware` (identificador heredado) pasa a una receta de `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` del ecosistema. No se crea un módulo router/otel ni se reinstala el middleware heredado. El ensamblador selecciona/pinea una versión de otelhttp compatible con sus APIs OTel y lo importa en su propia aplicación; este módulo no añade esa dependencia.

```go
handler := otelhttp.NewHandler(
    apiHandler,
    "api",
    otelhttp.WithTracerProvider(provider.TracerProvider()),
    otelhttp.WithMeterProvider(provider.MeterProvider()),
)
```

`provider` puede ser construido por telemetry/otel; también se pueden pasar providers API de otro constructor. No requiere globals. Es MIGRADO CON RUPTURA: naming, atributos, métricas, propagación y exclusiones son los de la instrumentación nativa elegida; no se promete equivalencia con la fachada/Recorder/Tracer heredados. La aplicación decide la posición respecto de autenticación, recuperación y rutas públicas. Su lifecycle/exporters se cierran explícitamente fuera del handler. Los exportadores pendientes de L9 no se consideran implementados por esta receta.
