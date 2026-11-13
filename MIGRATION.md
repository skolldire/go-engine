# Migration Guide

This guide lists the breaking changes and how to adapt.

## Experimental concurrency candidate (not released)

The standalone `github.com/skolldire/go-engine/concurrency` candidate replaces
its unpublished prototype APIs. It is not a stable/tagged v1 yet. No module
version or Go floor changes as part of this candidate.

| Before | Candidate API | Behavior |
|---|---|---|
| Config / Result in Map | StreamConfig / StreamResult | Streaming remains completion-order; cancellation may discard results |
| From(ctx, values...) returns channel | From returns (channel, error) | Nil context is rejected synchronously |
| Batch(size, wait, buffer) | BatchConfig + weight callback, StreamResult of batches | Explicit weight bounds and terminal invalid/oversized-item errors |
| NewLimiter(int), Acquire(ctx) | LimiterConfig, Acquire(ctx, weight) | FIFO weighted reservation and idempotent release |
| WorkerPool task map | NewPool / Submit(Job) / Future.Wait | Caller owns result collection; explicit admission and local live-ID uniqueness |
| WithTaskTimeout | PoolConfig.TaskTimeout | Running work keeps its slot until it returns; no detached task on timeout |
| WithResultTimeout | Future.Wait(ctx) / Shutdown(ctx, mode) | Wait timeout does not cancel the job; Future.Cancel is explicit |
| BatchWorkPool | Batch → Map or Pool submission | Streaming bounds instead of whole-workload materialization |
| Logger/metrics/result callbacks | Task/Future wrappers and Stats | No SDK or callbacks inside scheduler locks |
| Priorities | Not in concurrency v1 | No FIFO alias pretending to preserve priority semantics |

`pkg/utilities/task_executor` is frozen and deprecated, not a wrapper around the
new pool. It will be removed in the migration release. Its timeout may return
while user code continues running; consumers depending on that behavior must
migrate explicitly, not assume an equivalent alias. Its priority API is outside
concurrency v1. No behavior of the frozen implementation changes here.

The candidate's `Job.ID` is not durable idempotency; key serialization is local
to a process. Business consistency still belongs to the database/domain layer.
See [implementation evidence](docs/modules/concurrency.md) and
[contract migration table](docs/modularization/concurrency-api.md#10-migración-explícita-desde-las-apis-anteriores).

## `v0.30.0` — multi-module split and removal of `pkg/app`

The library is no longer a single module, and the old builder is gone. This is
the largest break so far, released without a deprecation window: nothing under
`pkg/app` or `pkg/config/viper` still exists.

### 1. Move to Go 1.27.4

Every module now declares `go 1.27.4`. Older toolchains are refused:

```
go: module github.com/skolldire/go-engine requires go >= 1.27.4
```

The floor is set to a patched release on purpose. A `toolchain` directive is
ignored in a dependency, so declaring a lower floor would let you build against
a Go with known vulnerabilities — `govulncheck` reported six as reachable in
1.26.5 (`net/http`, `encoding/asn1`, `golang.org/x/net/idna`). See
[`COMPATIBILITY.md`](COMPATIBILITY.md#go-version-policy).

### 2. Add the modules you actually use

```bash
go get github.com/skolldire/go-engine                    # core
go get github.com/skolldire/go-engine/aws                # only if you use AWS
go get github.com/skolldire/go-engine/database/redis     # only if you use Redis
go get github.com/skolldire/go-engine/preset/full        # or the everything preset
```

A pure HTTP service now downloads 55 `require` lines instead of 115.

### 3. Rewrite the import paths

| Before | After |
|---|---|
| `pkg/app`, `pkg/app/build`, `pkg/config/viper` | removed — see step 3 |
| `pkg/app/router` | `pkg/router` |
| `pkg/clients/rest` | `http/pkg/rest` |
| `pkg/utilities/telemetry` | `pkg/telemetry/otel` |
| `provider/{sqs,sns,ses,s3,ssm,dynamo,cognito,awsbase}` | `aws/provider/…` |
| `provider/{kafka,rabbitmq,grpcclient,grpcserver}` | `messaging/provider/…` |
| `provider/rest` | `http/provider/rest` |
| `provider/{redis,mongodb,memcached}` | `database/<engine>/provider/…` |
| `preset/aws` | `aws/preset` |
| `testutil.MockS3Client` / `MockSQSClient` | `aws/pkg/testutil` |
| `testutil.MockRestClient` | `http/pkg/testutil` |
| `testutil.MockRedisClient` | `database/redis/pkg/testutil` |

`pkg/testutil` keeps `MockLogger` and the context helpers — everything that
carries no adapter dependency.

### 4. Replace the builder with `engine.New`

```go
// before
eng, err := app.NewAppBuilder().WithContext(ctx).WithDynamicConfig().
    WithInitialization().WithRouter().Build()

// after
eng, err := engine.New(ctx, presetfull.Options()...)
```

The full mapping, including the 39 removed getters and the YAML keys that
changed meaning, is in [`docs/migration-engine.md`](docs/migration-engine.md).

### 5. `rest.Config.WithResilience` and `rest.Config.Resilience` were removed

The REST client composes retry and circuit breaking separately; a nil block
means that decorator is not installed at all.

```go
// before
rest.Config{BaseURL: url, WithResilience: true}

// after
rest.Config{
    BaseURL:        url,
    Retry:          &rest.RetryConfig{MaxRetries: 3},
    CircuitBreaker: &circuit_breaker.Config{Name: "api"},
}
```

Other clients (AWS, database, messaging) keep `WithResilience` — it is part of
`client.BaseConfig` and unchanged.

### 6. `telemetry.Config` was replaced by `otel.OTELConfig`

`pkg/utilities/telemetry` was a facade over `pkg/telemetry/otel`; the two are now
one package. `NewTelemetry`, `NewOperation`, `Metrics`, `Tracer` and `Telemetry`
moved verbatim; the constructor takes `OTELConfig`, whose keys are
`exporter_endpoint` and `sampling_rate` rather than `otel_endpoint` and
`sample_rate`. `provider/otel` rejects a configuration file still using the old
names rather than decoding them to empty values.

`NewTelemetryFrom(provider, cfg)` is the form to use when the provider is owned
by someone else: two providers mean two OTLP exporters, with whichever
registered last silently discarding the other's spans.

## Runtime correctness — stabilisation work

The changes below were introduced by phases 0–3 of
`docs/plan-mejoras-go-engine.md`.

### `client.Operation` now receives a context

`Operation` changed from `func() (any, error)` to
`func(context.Context) (any, error)`. The context passed in is the
timeout-bounded context managed by `Execute`, so operations must use it for I/O.

```go
// before
c.Execute(ctx, "op", func() (any, error) {
    return callAPI(ctx) // captured the caller's unbounded ctx
})

// after
c.Execute(ctx, "op", func(ctx context.Context) (any, error) {
    return callAPI(ctx) // uses the timeout-bounded ctx
})
```

`resilience.Service.Execute` changed its operation parameter the same way.

### `router.Service.Run` takes a context

```go
// before
engine.Router.Run()
// after
engine.Router.Run(ctx) // returns when ctx is cancelled, on signal, or on error
```

`Engine.Run()` is unchanged and now forwards the builder context.

### `health.Service` methods take a context

`IsReady()` → `IsReady(ctx)` and `GetStatus()` → `GetStatus(ctx)`. Pass the
request context in HTTP handlers.

### New `Engine.Close(ctx) error`

Resources created by the builder (Redis, Mongo, RabbitMQ, gRPC, Kafka,
telemetry) are now closed in LIFO order during router shutdown. You can also
`defer engine.Close(ctx)` if you do not use `Router.Run`.

## Configuration & security (phase 2)

### `aws.region` is only required when an AWS adapter is configured

Applications that do not use AWS no longer need to declare `aws.region`.

### Router: pprof is opt-in

Set `router.Config.EnablePprof = true` to expose `/debug/pprof` (never served
under the production profile). Previously it was exposed by default in non-prod.

### Router: trusted proxies

`TrustedProxies` now drives a spoofing-resistant real-IP middleware: forwarding
headers are trusted only when the direct peer is a configured proxy (IP or
CIDR). With no proxies configured, `X-Forwarded-For`/`X-Real-IP` are ignored.

### JWT middleware: issuer/audience required by default

`router.JWTAuthConfig` now fails closed (HTTP 500) when `Issuer` or `Audience`
is empty. To keep the previous behaviour, opt out explicitly:

```go
router.JWTAuthConfig{
    JWKSURL:            jwksURL,
    AllowEmptyIssuer:   true,
    AllowEmptyAudience: true,
}
```

Other JWT changes: RS256 is enforced, the JWKS HTTP status must be 2xx, the
body is size-limited, `use`/`alg` are validated, and a custom `HTTPClient` can
be injected.

### gRPC and OTLP transport security

- `grpc.Config` gained `TLS *TLSConfig`. Without it the client stays insecure
  (plaintext) as before.
- `otel.OTELConfig` gained `Insecure` and `Headers`. **OTLP now uses TLS by
  default**; set `Insecure: true` for a local collector.

### Viper is no longer a singleton

`viper.NewService` returns a fresh instance per call. If you relied on the
process-wide singleton, hold your own reference instead.

## API normalisation (phase 3)

### English method and error names

| Package | Before | After |
|---|---|---|
| `sqs` | `SendMsj` | `SendMessage` |
| `sqs` | `ReceiveMsj` | `ReceiveMessages` |
| `sqs` | `DeleteMsj` | `DeleteMessage` |
| `sqs` | `ErrEnviarMensaje` | `ErrSendMessage` |
| `sqs` | `ErrRecibirMensajes` | `ErrReceiveMessages` |
| `sqs` | `ErrEliminarMensaje` | `ErrDeleteMessage` |
| `sqs` | `ErrCrearCola` | `ErrCreateQueue` |
| `sqs` | `ErrEliminarCola` | `ErrDeleteQueue` |
| `sqs` | `ErrListarColas` | `ErrListQueues` |
| `sqs` | `ErrObtenerURLCola` | `ErrGetQueueURL` |
| `sns` | `PublishMsj` | `PublishMessage` |
| `grpc` | `NewCliente` | `NewClient` |

### `interface{}` → `any`

Signatures that used `interface{}` now spell it `any`. The two are identical, so
implementations and callers do not need changes; this is a cosmetic update.

## Experimental config candidate (not released)

`github.com/skolldire/go-engine/config` is an optional independent loader.
Existing engine/RawConfig/providers are unchanged; there is no compatibility
alias and no automatic migration. New applications explicitly provide FS/readers,
environment bindings and Resolver, seed their own tool defaults, Decode, Validate
and construct tools themselves.

| Legacy | Independent config |
|---|---|
| Viper file discovery/cwd/executable fallback | Explicit FS or Reader and Path; YAML/Viper adapters later |
| mapstructure tags and legacy matching, weak conversion | Same mapstructure tags/MatchName with mapstructure v2 strict hooks |
| comma-string to slice from every source | Typed lists from files; strict CSV only from env/resolved text |
| Missing section Decode no-op | ErrSectionMissing |
| os.Getenv at Decode, unresolved retained | Explicit captured lookup, resolve once per Load, missing error |
| Empty treated like absent for :- | Empty is present; fallback only for ErrReferenceNotFound |
| Raw mutable tree | Immutable snapshot, no raw getter |
| Implicit overwrite/merge behavior | Last source wins within same shape; cross-shape error, null deletes |
| Partially mutated target on error | Atomic Decode over seeded defaults |

Duplicate JSON keys and content after the root are rejected. Preserve default
values by decoding into DefaultConfig(), not a zero literal. Explicit 0/false are
not normalized. Maps merge with seeded values and lists replace. Conversion and
parser errors omit values; external dependency errors remain the caller's
confidentiality responsibility. No module is published by this extraction.

## D34 revised: mapstructure v2 and optional Viper source

The experimental config-tag decoder is replaced before release. Use
mapstructure tags (including explicit ,squash), not config tags. Untagged names
use the engine's case/underscore MatchName. config depends on mapstructure/v2
v2.5.0; Viper v1.21.0 lives only in config/viper. Neither replaces engine yet.

Before, engine loaded its own files and RawConfig.Decode performed weak decoding
and implicit environment expansion. DecodeNamed/InstanceNames remain legacy
assembly helpers rather than new config APIs. After:

```go
sourceCfg := viperSource.DefaultConfig()
sourceCfg.Files = []string{"application.yaml", "production.yaml"}
source, err := viperSource.New(sourceCfg, viperSource.Dependencies{FS: filesystem})
if err != nil { return err }
resolver, err := env.Resolver(capturedLookup)
if err != nil { return err }
loader, err := config.New(config.DefaultConfig(), config.Dependencies{
    Sources: []config.Source{source}, Resolver: resolver,
})
if err != nil { return err }
snapshot, err := loader.Load(ctx)
if err != nil { return err }
cfg := tool.DefaultConfig()
if err := snapshot.Decode("instances.orders", &cfg); err != nil { return err }
if err := cfg.Validate(); err != nil { return err }
```

The application supplies FS, explicit file paths and a captured immutable lookup
shared by env.Source and env.Resolver; it may read CONF_DIR explicitly if desired.
It owns construction and shutdown. A missing reference now fails instead of
reaching the client literally. Empty stays empty even with fallback, and embedded
references resolve. Missing sections fail while preserving defaults. File strings
such as "8080", "1" or CSV are not weakly converted; env/resolved text uses strict
formats and CSV with no empty elements. Unknown keys fail in every decoded section.
No mutable raw getter remains; no validation method is called implicitly.

config/viper intentionally keeps native lowercase/dotted-key/JSON float64
semantics. Use config/json for literal data keys and exact numeric input. Required
file handling is explicit: true requires every path, false ignores only not-exist.
The old reader may silently skip optional overlays; choose Required deliberately.
Comparative tests use a configcomparison build tag in pkg/engine without changing
its production code, imports or module requirements. Benchmarks compare the actual
legacy decoder with the new snapshot Decode using the same fixtures.

## Experimental typed client candidate (not released)

Independent github.com/skolldire/go-engine/client replaces no existing consumer
in this iteration. Typed Operation/Handler/Middleware and explicit Chain terminal
replace any-based calls and SafeTypeAssert in future migrations; Invocation uses
Component instead of Client. Nil middleware now fails. Context/timeouts/logging/
retry ownership is explicit; there is no mutable Use or implicit BaseConfig.
See [client migration](client/MIGRATION.md) for before/after and D35 preconditions.

Retry candidate: see [retry/MIGRATION.md](retry/MIGRATION.md). Legacy retries remain unchanged; typed retry requires explicit replay classification.

Circuitbreaker candidate: see [circuitbreaker/MIGRATION.md](circuitbreaker/MIGRATION.md). Legacy circuit_breaker/resilience remain unchanged; new classification and recovery semantics are explicit.

Ratelimit candidate: see [ratelimit/MIGRATION.md](ratelimit/MIGRATION.md). Local rate admission is distinct from concurrency limits and SDK throttling policies.
