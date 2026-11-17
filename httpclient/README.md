# httpclient

Experimental D53 candidate, not released. Go 1.27.4, standard library only, tests included.

An outbound HTTP client for one upstream: a bounded transport, a response size limit, a whole-request timeout, explicit redirects and a rule that keeps every call on the configured origin. It has no retry, circuit breaker, rate limit, logging, metric or trace of its own. Those are composed from outside (see [Composition](#composition)).

```go
cfg := httpclient.DefaultConfig()
cfg.BaseURL = "https://inventory.internal/api/v1"
cfg.MaxResponseBytes = 1 << 20     // required: no default is safe for every upstream
cfg.Transport.MaxConnsPerHost = 16 // required, for the same reason
c, err := httpclient.New(cfg, httpclient.Dependencies{})
// handle err

req, err := c.NewRequest(ctx, http.MethodGet, "/items/7", nil)
// handle err
resp, err := c.Do(ctx, req)
if err != nil {
	return err
}
defer resp.Body.Close() // always: see "The caller closes the body"
if err := httpclient.CheckStatus(resp); err != nil {
	return err // *httpclient.StatusError: the code, never the body or the URL
}
data, err := io.ReadAll(resp.Body)
```

## Configuration

`Config.Validate` checks fields in declaration order and completes nothing: a zero literal is rejected, not replaced by a default. Start from `DefaultConfig()`. All fields carry snake_case `mapstructure` and `json` tags.

| Field | Default | Rule |
|---|---|---|
| `BaseURL` | none | required; `http` or `https`, a host, an optional plain path prefix |
| `RequestTimeout` | 10s | > 0 |
| `MaxResponseBytes` | none | required, > 0 |
| `FollowRedirects` | false | |
| `MaxRedirects` | 0 | > 0 with `FollowRedirects`, 0 without it |
| `Transport.ConnectTimeout` | 5s | > 0 |
| `Transport.ResponseHeaderTimeout` | 5s | > 0 |
| `Transport.IdleTimeout` | 90s | > 0 |
| `Transport.MaxIdleConns` | 100 | > 0 |
| `Transport.MaxIdleConnsPerHost` | 10 | > 0 |
| `Transport.MaxConnsPerHost` | none | required, > 0 |
| `Transport.ProxyURL` | none | optional; `http`, `https`, `socks5` or `socks5h` with a host |

`BaseURL` must not contain userinfo, a query or a fragment, and its path must be a plain prefix: no `.`/`..`, empty or percent-encoded segments. A trailing slash is ignored.

`ConfigError{Field, Reason}` wraps `ErrInvalidConfig`. It names the Go field and a fixed reason and never includes the rejected value, since a base or proxy URL can embed credentials.

## Construction and transport

`New` is pure: it validates and assembles, with no connection, file or goroutine.

**Own transport** (no `Dependencies.RoundTripper`): an `*http.Transport` built from `TransportConfig` with a `net.Dialer` using `ConnectTimeout`, `ForceAttemptHTTP2` and the connection limits. `ConnectTimeout` also bounds the TLS handshake. A proxy is used only when `ProxyURL` is set; `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` are never read. `Dependencies.TLSConfig` is cloned; the module reads no certificate files.

**Borrowed transport** (`Dependencies.RoundTripper` set): it is used exactly as given and never modified or closed. `Config.Transport` must then be the zero value and `TLSConfig` must be nil; anything else is a `ConfigError`, because those settings could not be applied to a transport this module does not own. Note that `DefaultConfig()` fills `Transport`, so reset it:

```go
cfg.Transport = httpclient.TransportConfig{}
c, err := httpclient.New(cfg, httpclient.Dependencies{RoundTripper: shared})
```

A zero `Transport` passes `Validate` (it is how a borrowed transport is declared) and is rejected by `New` when no `RoundTripper` comes with it. With a borrowed transport, connection limits, proxy and TLS are whatever that transport does, including reading the environment if it was built that way.

**`WrapTransport`**, when set, is called once on the final transport, own or borrowed, for example `otelhttp.NewTransport`. Returning nil is a `ConfigError`. The wrapper sits inside the protections below and sees every hop of a redirect chain.

`HTTPClient()` always returns the same `*http.Client`. It is shared: **do not modify it** (timeout, transport, redirect policy or jar). `CloseIdleConnections` closes idle connections of the module's own transport and does nothing to a borrowed one.

## What every request goes through

The protections live in an internal `RoundTripper` and in the `*http.Client` returned by `HTTPClient()`, so they also bind code that uses that client directly, such as the [resty adapter](resty/README.md).

**Origin.** A request must target the scheme, host and port of `BaseURL`. Anything else is `ErrOriginNotAllowed`, returned before any network activity, which is what stops a caller-influenced URL from reaching an unintended destination (SSRF). Default ports are equivalent to their explicit form and host comparison is case-insensitive. A URL with userinfo is refused even on the right host. There is no IDNA or trailing-dot folding: another spelling of the same host is refused. The rule compares URLs; it does not pin DNS answers, and it does not constrain where a proxy or a borrowed transport connects. It checks the request URL, not `Request.Host`: a caller that overrides the `Host` header changes what the upstream sees, not where the connection goes.

- `Do` resolves a relative URL by appending it to the base path (`/api/v1` + `/items` → `/api/v1/items`); this is concatenation, not RFC 3986 resolution. Dot segments in a relative path, encoded or not, are `ErrInvalidCall`.
- `NewRequest` accepts relative paths only, with an optional query. An absolute or scheme-relative value matches both `ErrInvalidCall` and `ErrOriginNotAllowed`. Escape untrusted path segments with `url.PathEscape`.
- `HTTPClient()` used directly needs absolute URLs; a relative one is `ErrInvalidCall`.

**Timeout.** `RequestTimeout` covers the whole request: connecting, every redirect hop and reading the body. Its context is released when the body reaches EOF, fails, or is closed, and never when headers are returned. If the caller's context has an earlier deadline, that one applies. Expiry matches `context.DeadlineExceeded` and reports `Timeout() == true`, while waiting for headers and while reading the body.

**Response size.** If `Content-Length` exceeds `MaxResponseBytes`, the body is closed and `ErrResponseTooLarge` is returned with no response. If the length is unknown, the body yields up to `MaxResponseBytes` bytes and then `ErrResponseTooLarge`; it is never truncated silently. A body of exactly the limit is accepted. The limit counts decoded bytes when the transport decompresses gzip on its own; if you set `Accept-Encoding` yourself, it counts bytes on the wire and decoding is yours to bound. `HEAD` responses are exempt from the `Content-Length` check. Response headers keep net/http's own limit.

**Redirects.** With `FollowRedirects=false` a 3xx is returned as it arrived. With `true`, at most `MaxRedirects` are followed; one more is `ErrRedirectNotAllowed`, as is any hop from `https` to `http` or to a non-HTTP scheme. A redirect to another origin drops `Authorization`, `Proxy-Authorization` and `Cookie`, including to a subdomain or another port, which is stricter than net/http; once a chain has left the first origin they stay dropped even if it returns. Redirects are not checked against `BaseURL`: enabling them is the caller's decision to follow the upstream elsewhere. That includes the request body: a 307 or 308 resends it to the new location, as net/http does, even on another origin. When a redirect is refused `Do` returns a nil response; `HTTPClient()` used directly behaves as net/http does and also returns the last response, already closed.

**Status.** A non-2xx status is not an error: `Do` returns the response and a nil error. `CheckStatus(resp)` returns nil for 2xx and `*StatusError{StatusCode, Response}` otherwise, without reading or closing the body. Its message is `httpclient: unexpected status <code>`: no body, no URL. A nil response is `ErrInvalidCall`.

`Do(ctx, req)` uses `ctx` in place of the request's own context and leaves the caller's request unmodified. A nil `ctx` is `ErrNilContext`; a nil request is `ErrInvalidCall`. `NewRequest` with a `[]byte` body sets `ContentLength` and `GetBody`, so the request can be rebuilt for another attempt or a 307/308; the slice is not copied and must not be modified while in use. No header is added beyond what net/http adds. `Client` is safe for concurrent use.

Transport failures are net/http's `*url.Error`, whose text includes the request URL with its query. Do not put secrets in query strings, or avoid logging error text (`client/logging` omits it).

## The caller closes the body

Always close `resp.Body`, on every path, including non-2xx responses. An unclosed body keeps its connection out of the pool and its request context alive. With `MaxConnsPerHost` reached, the next call waits until its own context expires; the test suite demonstrates exactly this with a single-connection client.

## Composition

Retry, breaker and rate limit belong to `resilience` (one policy owns the budget); logging and tracing to `client/logging` and `client/otel`. The shape below is exercised by `examples/standalone/httpclient/composed`.

```go
// One attempt. The request is created inside the operation, so every
// attempt sends the whole body under its own context.
attempt := client.Operation[[]byte](func(ctx context.Context) ([]byte, error) {
	req, err := c.NewRequest(ctx, method, path, body)
	if err != nil {
		return nil, &attemptError{method: method, err: err}
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, &attemptError{method: method, err: err}
	}
	defer resp.Body.Close() // before the attempt context is cancelled
	if err := httpclient.CheckStatus(resp); err != nil {
		return nil, &attemptError{method: method, err: err}
	}
	return io.ReadAll(resp.Body)
})
resilient, err := resilience.Wrap(attempt, policy, resilience.Dependencies{Classify: classify})
// client/logging or client/otel wrap the resilient operation, outside:
data, err := client.Do(ctx, info, resilient, observe)
```

The classifier is the caller's. A sound default:

- `GET`, `HEAD`, `PUT`, `DELETE`: retry on transport errors and on 429, 502, 503, 504.
- `POST`, `PATCH`: retry only when the request carries an `Idempotency-Key` the server honors, as an explicit decision.
- A 500 or a transport error on a `POST` without a key is `Stop` or `Reconcile`, never `Retry`: the side effect may have been applied.
- `ErrOriginNotAllowed`, `ErrResponseTooLarge`, `ErrRedirectNotAllowed` and `ErrInvalidCall` never improve with another attempt.

Known limitation: a classifier can read `Retry-After` from `StatusError.Response`, but `retry` does not wait according to it (D36). Backoff follows the retry configuration only.

For per-request spans or metrics at the HTTP layer, inject an instrumenting transport through `WrapTransport`.

See [MIGRATION](MIGRATION.md), [RELEASE_NOTES](RELEASE_NOTES.md) and the independent consumers under `examples/standalone/httpclient`. The proposed v0.1.0 is unapproved and unreleased; this is not published stability.
