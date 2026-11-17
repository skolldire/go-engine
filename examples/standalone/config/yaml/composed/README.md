# config/yaml composed

`config/yaml`, `config` and `validation`, all real modules: settings are read from YAML, decoded into a struct and validated. The generic checker requires a composed consumer for every production module; this one shows the division of work. `config/yaml` rejects malformed input, `config` decodes, and business rules belong to the application, here through `validation`.

A well-formed file with an invalid address and an out-of-range port fails validation with the settings named by path (`listeners[1].port`) and without their values.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From the repository root: `scripts/check-module.sh config/yaml consumers`.
