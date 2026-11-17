# Migration — experimental, not released

The legacy `http/pkg/rest` client and `http/provider/rest` stay unchanged. There are no aliases or compatibility bridges. Read [../MIGRATION.md](../MIGRATION.md) first: timeout, limits, origin, retry, breaker and logging all move to `httpclient` and its composition, not to this adapter.

Before:

```go
c := rest.NewClient(ctx, rest.Config{BaseURL: base, Retry: &rest.RetryConfig{MaxRetries: 3}}, log)
resp, err := c.Get(ctx, "/items/7", nil) // err != nil on a non-2xx, with body text inside
```

After:

```go
hc, err := httpclient.New(cfg, httpclient.Dependencies{})
// handle err
c, err := httpresty.New(httpresty.Dependencies{HTTPClient: hc})
// handle err
resp, err := c.Get(ctx, "/items/7", nil) // err == nil on a non-2xx
// handle err
err = httpresty.CheckStatus(resp)        // *httpclient.StatusError, code only
```

| Legacy `rest.Service` | `httpclient/resty` |
|---|---|
| `NewClient(ctx, cfg, logger)` | `New(Dependencies{HTTPClient})`; no `Config` |
| `R(ctx)` | `R(ctx)`, same resty builder |
| `Get/Post/Put/Patch/Delete(ctx, endpoint, ...)` | same shapes; `endpoint` is a path under `BaseURL` |
| non-2xx: response **and** error with a body preview | response and nil error; `CheckStatus` for an error without the body |
| `Retry` block driving resty's retry | none; resty retry is off and cannot be enabled. Use `resilience` |
| `CircuitBreaker` block | none; `circuitbreaker` through `resilience` |
| `WithLogging`, `EnableLogging` | none; `client/logging` |
| absolute URL to any host through `R(ctx)` | only the httpclient's origin |

**Breaking: non-2xx no longer returns an error.** Code that relied on `err != nil` to detect a failed status must call `CheckStatus`. Code that read the status out of the error text must use `errors.As` with `*httpclient.StatusError`.

`RetryableStatus`, `IsIdempotent`, `ConservativeRetryCondition`, `AllowNonIdempotentRetryCondition`, `RetryAfter` and `ErrCircuitOpen` have no counterpart: the replay policy is the classifier you give `resilience`.
