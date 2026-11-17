# httpclient/resty

Experimental D53 candidate, not released. Go 1.27.4; depends on `httpclient` and the repository's existing `github.com/go-resty/resty/v2` v2.17.2.

An optional adapter that puts resty's request builder and JSON handling on top of an [`httpclient.Client`](../README.md). It has no configuration of its own: the transport, origin rule, response limit, timeout and redirect policy are the httpclient's, and are inherited because resty runs over `HTTPClient()`.

```go
base, err := httpclient.New(cfg, httpclient.Dependencies{})
// handle err
c, err := httpresty.New(httpresty.Dependencies{HTTPClient: base})
// handle err

var created Order
resp, err := c.R(ctx).SetBody(Order{SKU: "bolt"}).SetResult(&created).Post("/orders")
if err != nil {
	return err
}
if err := httpresty.CheckStatus(resp); err != nil {
	return err // *httpclient.StatusError
}
```

## API

- `New(Dependencies{HTTPClient})`: the client is required and borrowed; nil, or one not built by `httpclient.New`, is an `*httpclient.ConfigError`. `New` is pure. Closing idle connections remains the httpclient's job.
- `R(ctx)` returns resty's `*resty.Request` bound to `ctx`, for path and query parameters, typed results and multipart.
- `Get`, `Post`, `Put`, `Patch`, `Delete` cover the common case with a path, an optional body and headers. A nil `ctx` is `httpclient.ErrNilContext`.
- `CheckStatus(resp)` returns nil for 2xx and `*httpclient.StatusError` otherwise, the same type `httpclient.CheckStatus` returns, so one classifier serves both clients. A nil response is `httpclient.ErrInvalidCall`.

Paths are appended to the httpclient's `BaseURL`. An absolute URL is accepted only on that origin; any other is `httpclient.ErrOriginNotAllowed` before a connection is made.

## Behavior to know

**A non-2xx status is not an error.** The response comes back with a nil error, as in `httpclient`. This breaks with the legacy `http/pkg/rest` verb helpers, which returned an error carrying part of the body. Call `CheckStatus`.

**resty does not retry.** The retry count is zero and no retry condition is installed. The underlying `*resty.Client` is deliberately not exposed, so resty's retry loop cannot be switched on next to `resilience`; a request-level `AddRetryCondition` has no effect. Compose retries with `resilience` around the call (see `examples/standalone/httpclient/resty/composed`).

**resty buffers the response body.** `MaxResponseBytes` still applies: an oversized response fails with an error matching `httpclient.ErrResponseTooLarge`. To stream, use `R(ctx).SetDoNotParseResponse(true)` and read and close `resp.RawBody()` yourself; the limit and the timeout apply to that body too.

**`Accept-Encoding` cannot be set by hand.** The transport negotiates gzip and the size limit then counts decoded bytes. Setting the header yourself would make resty inflate the body after the limit was applied, so such a request is refused with `httpclient.ErrInvalidCall`.

**resty's own behavior is otherwise unchanged**, including the headers it adds (`User-Agent`, and `Content-Type` inferred from the body). The adapter gives resty a copy of the shared `*http.Client`, so request options that alter the client they hold do not reach the one `httpclient` shares.

See [MIGRATION](MIGRATION.md) and [RELEASE_NOTES](RELEASE_NOTES.md). The proposed v0.1.0 is unapproved and unreleased; this is not published stability.
