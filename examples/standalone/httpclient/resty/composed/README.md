# httpclient/resty composed

A resty call retried by `resilience`, with real `httpclient`, `httpclient/resty`, `client`, `resilience` and `retry` against an `httptest` server on loopback. The generic checker requires a composed consumer for every production module; this one shows the property that matters for the adapter: resty sends once per attempt and the classifier is the only replay policy.

A `GET` answered 503 and then 200 succeeds after two requests; a 410 is returned after one. Both clients report a non-2xx as `*httpclient.StatusError`, so the classifier does not depend on resty.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From the repository root: `scripts/check-module.sh httpclient/resty consumers`.
