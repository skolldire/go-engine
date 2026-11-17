# config/yaml minimal

`config` and `config/yaml` with an in-memory filesystem (`testing/fstest`). A base file and an override are loaded in order, with an optional third file that does not exist, and decoded over a `defaults()` value.

The files use an anchor, an alias and a merge key whose explicit key wins; the override replaces a list, merges a map, removes a key with `null` and writes a port in hexadecimal. `run` then shows the strict side: a duplicate key is an error naming `primary.password` and its line without the value, and a quoted number is not converted into an `int`.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From the repository root: `scripts/check-module.sh config/yaml consumers`.
