# httpclient/resty minimal

resty over an `httpclient`, against an `httptest` server on loopback: a JSON request and typed result, `CheckStatus` on a non-2xx, and the origin rule inherited from the httpclient. Local replaces point five levels up to `httpclient` and `httpclient/resty`; resty v2.17.2 comes through the adapter.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From the repository root: `scripts/check-module.sh httpclient/resty consumers`.
