# Migration Guide

This guide lists the breaking changes introduced by the stabilisation work
(phases 0–3 of `docs/plan-mejoras-go-engine.md`) and how to adapt.

## Runtime correctness (phase 1)

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
