# Independent logging + client composition

The consumer defines its own I/O adapter with an injected in-memory transport and a caller-owned middleware. Exactly one logical-call observation contains fixed labels and a success flag, without payload or returned error text. Result and error identities are preserved. No public client/logging wrapper is implemented.

Only logging, client and stdlib. Each replace points four levels up to the corresponding real module. v0.0.0 placeholders are local development versions.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

Run `scripts/check-module.sh logging consumers` from the root for the catalog-derived import/module/graph/replace checks and offline execution.
