# auth/jwt composed

In-memory JWKS, no socket or service. Caller-generated fixtures are for verification tests, not a token issuer API. Local replaces resolve only the selected repository modules. Run `go run .` and `go test -race ./...` with GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly. See go.mod for exact paths and dependency versions.
