# Independent logging consumer

Only logging and the standard library. The local replace points four levels up to logging; v0.0.0 is an unpublished development placeholder, not a release.

From this directory:

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From the repository root, `scripts/check-module.sh logging consumers` validates both consumers, their modules, imports, module graph and replaces using the catalog's derived boundary. No framework, filesystem, sockets or services are initialized.
