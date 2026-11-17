# validation minimal

Uses only `validation`: two rules registered on the instance (one reading the context given to `Struct`), a built-in name refused at construction, failures reported as path, rule and parameter in declaration order, and a check that the submitted password is nowhere in the error. No global validator exists to configure. The single local replace points four levels up to the module; the validator library arrives through it.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From the repository root: `scripts/check-module.sh validation consumers`.
