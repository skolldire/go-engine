# telemetry/otel

Explicit native OpenTelemetry trace and metric providers. Candidate, not published. Go 1.27.4; OTel v1.47.0, matching the repository. No exporter, transport, collector, global installation or framework dependency is created.

```go
cfg := otelprovider.DefaultConfig()
cfg.ServiceName = "orders"
p, err := otelprovider.New(cfg, otelprovider.Dependencies{})
if err != nil { return err }
_, span := p.Tracer("orders").Start(ctx, "lookup")
span.End()
return p.Shutdown(ctx)
```

Import as `otelprovider "github.com/skolldire/go-engine/telemetry/otel"`. Inject a caller-built sdktrace.SpanProcessor and/or sdkmetric.Reader for export/collection. Without either dependency its signal is not exported; this does not promise no recording or allocations. The caller constructs exporters and batch/periodic components. No managed queue/batch/flush settings exist here.

## Configuration

All literals are validated unchanged; call DefaultConfig explicitly to seed defaults. Config has json/mapstructure snake_case tags.

| Field | Default | Rule |
|---|---|---|
| ServiceName | empty | required, no normalization |
| ServiceVersion | empty | optional |
| Environment | empty | optional |
| SamplingRatio | 1 | finite, 0..1 |
| MaxAttributes | 128 | positive |
| MaxAttributeValueLength | 4096 | positive; SDK characters for string span attributes |

The sampler is ParentBased(TraceIDRatioBased(ratio)). Ratio controls roots; a sampled parent can record with ratio zero. Resource uses semconv/v1.43.0.SchemaURL, service.name, optional service.version and deployment.environment.name. Config service fields are not truncated.

## Resource environment merge (D45.4 amended)

OTel v1.47.0 WithResource merges the environment Resource into the explicitly supplied Resource for **both traces and metrics**. Config fields that are present take priority; extra environment keys remain. Empty optional Config fields do not override environment keys. This module neither changes the process environment nor adds a facade to suppress native behavior.

Variables that influence the final Resource:

- `OTEL_RESOURCE_ATTRIBUTES`: comma-separated key/value attributes, including additional keys and optional service/environment attributes.
- `OTEL_SERVICE_NAME`: the SDK environment service.name, taking precedence over service.name in OTEL_RESOURCE_ATTRIBUTES; the required Config.ServiceName then overrides both.

For example, with OTEL_RESOURCE_ATTRIBUTES=`service.name=env,extra.key=value` and Config.ServiceName=`orders`, both signals contain service.name=`orders` and extra.key=`value`. No host/process/container detectors or resource.Default merge are requested by this module.

Sampler and native SpanLimits are passed explicitly, so OTEL_TRACES_SAMPLER, OTEL_TRACES_SAMPLER_ARG, OTEL_SPAN_ATTRIBUTE_COUNT_LIMIT, OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT, OTEL_SPAN_EVENT_COUNT_LIMIT, OTEL_SPAN_LINK_COUNT_LIMIT, OTEL_EVENT_ATTRIBUTE_COUNT_LIMIT and OTEL_LINK_ATTRIBUTE_COUNT_LIMIT do not change their effective policy. Tests use conflicting t.Setenv values. Metric exemplars are explicitly TraceBasedFilter; cardinality limit is explicitly disabled (0), overriding OTEL_METRICS_EXEMPLAR_FILTER and OTEL_GO_X_CARDINALITY_LIMIT. Metrics cardinality therefore remains the caller's responsibility.

The SDK evaluates some environment defaults before applying explicit options, and may report invalid settings through its global error handler or populate internal caches. These options isolate effective sampling/limits, not all internal SDK reads or diagnostics. Injected readers/processors/exporters can have their own environment policy and background activity, outside this module's control.

## Native limits, without facades

WithRawSpanLimits fixes span/event/link attribute counts to MaxAttributes, event and link counts to 128 each, and native attribute value depth to 64. MaxAttributeValueLength counts SDK string characters (Unicode code points), **not UTF-8 bytes**; one `界` is one character and three bytes. String-slice elements receive the native span-attribute length limit. SDK behavior for other native value types is preserved.

Approved D45.3 scope correction: **events, links and RecordError exception events have no value-length limit in SDK v1.47.0**. Their count/depth limits still apply. Start, SetAttributes, SpanFromContext and tracers obtained from TracerProvider all use the same native SDK limits. No custom Tracer/Span wrapper exists, so native context access and third-party instrumentation remain compatible. This is not secret redaction, a metric-cardinality limit or a bound on total memory/callback cost.

## Ownership and shutdown

New validates Config and builds the Resource before registering dependencies. SDK constructors return no error. On normal successful return, sole ownership of the optional processor/reader transfers to Provider; do not share or reuse them with another provider. Validation failure does not register or close dependencies. Invalid or panicking injected components follow SDK behavior; no recover or rollback promise is added.

Shutdown(nil) returns ErrNilContext. The first Shutdown calls both SDK providers with its context and joins their errors. Concurrent/subsequent calls do not retry: they await and return the cached first result, or return their own context error/cause while waiting. A completed cached result wins over an already canceled waiting caller. When completion and cancellation race, either observation can win. No module lock is held over callbacks and no goroutine abandons a noncooperative close.

“Closed” means the SDK close was invoked, not guaranteed flush/disposal. In particular, native trace shutdown marks itself shut before checking a canceled context and can skip processor Shutdown; it cannot be retried. The first error remains visible. Use a suitable live shutdown context and inspect the result. SDK trace/meter callbacks can panic or Goexit; these propagate, both signals are still attempted through defer, and waiters receive an explicit incomplete-close error. A dependency must not recursively wait for its owner's Shutdown.

TracerProvider() and MeterProvider() expose API interfaces, not SDK types in their signatures. Previously acquired instruments follow native SDK post-shutdown behavior. No ForceFlush, Close, lifecycle manager or SDK getter is added.

## Globals are an assembler decision

Prefer passing API providers explicitly to instrumentation. Future wrappers depend only on trace.TracerProvider/metric.MeterProvider, never this module or the SDK (D45.8).

If the application chooses globals, do this **before creating instrumentations**:

```go
otel.SetTracerProvider(p.TracerProvider())
otel.SetMeterProvider(p.MeterProvider())
```

Here `otel` is go.opentelemetry.io/otel. Stop using instrumentation before Shutdown. Saving and resetting global slots cannot undo already delivered provider/tracer/meter references or the SDK's one-time delegation of default proxies. There is no InstallGlobal/restore ownership registry. The recipe is tested in a subprocess so it cannot contaminate other tests.

## Verification

From the repository root: `scripts/check-module.sh telemetry/otel deps`, `consumers`, `quick`, `stress`, `fuzz`, `bench`. Prefix TELEMETRYOTEL controls campaign time/output overrides. Fixtures use captured spans, ManualReader and stdlib gates; no sockets/collector/export transport. See RELEASE_NOTES.md for executed commands, platform scope and pending distribution evidence.
