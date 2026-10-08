# Migration Guide

This guide lists the breaking changes and how to adapt.

## `v0.30.0` — multi-module split and removal of `pkg/app`

The library is no longer a single module, and the old builder is gone. This is
the largest break so far, released without a deprecation window: nothing under
`pkg/app` or `pkg/config/viper` still exists.

### 1. Move to Go 1.26.6

Every module now declares `go 1.26.6`. Older toolchains are refused:

```
go: module github.com/skolldire/go-engine requires go >= 1.26.6
```

The floor is set to a patched release on purpose. A `toolchain` directive is
ignored in a dependency, so declaring a lower floor would let you build against
a Go with the six vulnerabilities `govulncheck` reports as reachable in 1.26.5
(`net/http`, `encoding/asn1`, `golang.org/x/net/idna`). See
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
