# httpclient minimal

Uses only `httpclient` against an `httptest` server on loopback: `NewRequest`, `Do`, `CheckStatus`, the response size limit and the origin rule. No other module, service or external network. The single local replace points four levels up to the module; v0.0.0 is a development placeholder.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From the repository root: `scripts/check-module.sh httpclient consumers`.
