# Migration — experimental, not released

The legacy `http/pkg/rest` client and `http/provider/rest` stay unchanged and supported. There are no aliases, compatibility bridges or deprecations; nothing is migrated automatically.

| Legacy `http/pkg/rest` | `httpclient` |
|---|---|
| `rest.NewClient(ctx, cfg, logger)` returning a `Service` over resty | `httpclient.New(cfg, deps)`; resty is optional through `httpclient/resty` |
| `TimeOut` zero normalized to 10s | `RequestTimeout` validated, zero rejected; start from `DefaultConfig()` |
| resty retry (`RetryConfig`, `Conservative`/`AllowNonIdempotent`, `Retry-After`) | `resilience.Wrap` with a classifier the caller writes |
| circuit breaker as a `RoundTripper` inside resty's retry loop | `circuitbreaker` through `resilience`, per attempt |
| non-2xx returned as an error holding status and the first 200 body characters | response with a nil error, then `CheckStatus`; no body or URL in errors |
| `EnableLogging` / `WithLogging` | `client/logging` around the logical call |
| resty default transport: no per-host or response size limits | `TransportConfig` and required `MaxResponseBytes` |
| any URL accepted | one origin; anything else is `ErrOriginNotAllowed` |
| redirects followed by resty's default policy | off unless `FollowRedirects`, bounded by `MaxRedirects` |
| proxy from the environment | only `Transport.ProxyURL` |

## Construction and timeout

Before:

```go
c := rest.NewClient(ctx, rest.Config{BaseURL: "https://inventory.internal", TimeOut: 0}, log) // 0 became 10s
```

After:

```go
cfg := httpclient.DefaultConfig() // RequestTimeout 10s, stated rather than implied
cfg.BaseURL = "https://inventory.internal"
cfg.MaxResponseBytes = 1 << 20
cfg.Transport.MaxConnsPerHost = 16
c, err := httpclient.New(cfg, httpclient.Dependencies{})
```

`New` returns an error instead of normalizing: a zero `RequestTimeout`, a missing `MaxResponseBytes` or a missing `MaxConnsPerHost` is a `ConfigError`. TLS material is not read from files; build a `*tls.Config` and pass it as `Dependencies.TLSConfig`.

## Status and errors

Before, the verb helpers returned an error for a non-2xx, and its text carried up to 200 characters of the response body:

```go
resp, err := c.Get(ctx, "/items/7", nil)
// err: "HTTP 404: 404 Not Found - {\"customer\":\"...\"}"  — the body ends up in logs
```

After, the status is a response and the error names only the code:

```go
resp, err := c.Do(ctx, req)
if err != nil {
	return err // transport, timeout, origin or size
}
defer resp.Body.Close()
if err := httpclient.CheckStatus(resp); err != nil {
	return err // "httpclient: unexpected status 404"
}
```

Code that parsed the old error text must switch to `errors.As(err, &statusErr)` and read `statusErr.StatusCode`. The response is still available as `statusErr.Response` when the body is needed deliberately.

## Retry

Before, resty retried inside the client, replaying idempotent methods on 429/500/502/503/504/507 and honoring `Retry-After`:

```go
rest.Config{Retry: &rest.RetryConfig{MaxRetries: 3, RetryNonIdempotent: false}}
```

After, the client sends once and `resilience` owns the attempts, with the request built per attempt so the body is sent whole each time:

```go
attempt := client.Operation[[]byte](func(ctx context.Context) ([]byte, error) { /* NewRequest, Do, CheckStatus, close */ })
op, err := resilience.Wrap(attempt, policy, resilience.Dependencies{Classify: classify})
```

Differences to plan for: the classifier is yours and decides by method and status; a 500 is no longer retried by default, since on a `POST` it can hide an applied side effect; `POST`/`PATCH` need an `Idempotency-Key` and an explicit decision; `Retry-After` can be read by the classifier but `retry` does not wait according to it (D36).

## Circuit breaker

Before, a breaker `RoundTripper` sat inside resty's retry loop and counted 429/5xx as failures. After, pass a `circuitbreaker` as `resilience.Dependencies.Breaker`; it runs per attempt with its own classifier. There is no breaker inside the transport.

## Logging

Before: `rest.Config{EnableLogging: true}` or `WithLogging(true)` logged through `BaseClient`. After: build `client/logging` middleware and pass it to `client.Do` outside the resilient operation, for one record per logical call. `httpclient` logs nothing.

## Transport limits and origin

Before, resty's default transport had no per-host connection limit and buffered a response of any size. After, `Transport.MaxConnsPerHost` and `MaxResponseBytes` are required, and an oversized response is `ErrResponseTooLarge`.

Before, the verb helpers concatenated `BaseURL + endpoint` and `R(ctx)` accepted any absolute URL. After, an absolute URL on another scheme, host or port is `ErrOriginNotAllowed` before any connection. A service that calls several upstreams builds one client per upstream.

`http/provider/rest` has no counterpart here: this module is not an `engine.Provider` and reads no engine configuration section.
