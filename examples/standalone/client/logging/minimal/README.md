# Independent client/logging consumer

Only client/logging, client and stdlib. Native slog JSON plus an application-owned slice Logger prove no backend dependency. Covers defaults, literal false, Do/Chain, original partial result/error, validation and disabled emission. The module logging never appears in this consumer's graph.

The two local replaces point five levels up to the corresponding catalog directories. v0.0.0 is a development placeholder, not a published version.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From root: scripts/check-module.sh client/logging consumers verifies both consumers' module/import/graph/replace boundaries and runs them offline.
