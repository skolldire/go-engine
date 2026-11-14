# API-only client/otel consumer

Uses client/otel, client and OTel API only, with API no-op providers and a local capture provider. No provider SDK or telemetry/otel is needed. Local replaces ../../../../../client and ../../../../../client/otel are development-only.

GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...

The generic checker measures imports, module graph and exact replaces. New validation, typed partial values/errors, Do and reusable Chain are exercised without a framework.
