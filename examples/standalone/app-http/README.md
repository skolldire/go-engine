# HTTP application without engine

Run with Go1.27.4, GOWORK=off and the local replaces in go.mod:

`go run . -api-listen 127.0.0.1:8080 -operations-listen 127.0.0.1:8081`

POST /orders accepts id/item/amount_cents; GET /orders/{id} retrieves memory orders. Operations are separate: /ping,/live,/ready,/deps; pprof is off. Main owns signal.NotifyContext and calls Application.Run. Logger output is borrowed stdout and the memory store owns no closeable resource. The private build helper explicitly assembles health, logging, public error mapping and the preset; it is not an engine/registry.

Tests use ResponseRecorder and ephemeral loopback RunListeners. No Redis, SDK, environment/profile lookup or service is simulated. The memory store is concurrency-safe. Error/panic causes never enter public responses; request IDs correlate safe responses. Caller recovery cannot undo committed bytes.

Offline checks: GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race -count=5 ./... . Full lifecycle checks still require loopback permission; see preset RELEASE_NOTES.
