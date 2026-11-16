# Independent router/errors consumer

Only router/errors and stdlib. Local replace ../../../../../router/errors is development-only. Explicit caller classification, known/unknown errors, RFC9457 JSON, request ID, validation, fallback and one-attempt short-write handling use in-memory writers only.

Run go run . and go test -race ./... with GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly. No router, SDK, framework, listener or environment lookup is required.
