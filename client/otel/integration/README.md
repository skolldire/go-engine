# client/otel real SDK harness

Nonpublished harness, GOWORK=off, with direct API/SDK dependencies declared in the catalog. Local replaces ../.. (client), .. (client/otel), ../../../telemetry/otel (provider factory) point only at used modules. The production wrapper and its tests remain API-only.

Run go test -race -count=20 ./... with default GOMAXPROCS, 1 and 4; go vet ./...; CLIENTOTEL_STRESS=1 go test -race -run '^TestStress' -count=3 -v ./...; go test -run '^$' -bench . -benchmem -benchtime=100ms -count=3 ./.... Set GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly and prepare the existing-version cache first.

SpanRecorder and ManualReader prove native parenting/status/sampling, one histogram observation per normal call, seconds with synctest, exemplar correlation, explicit provider ownership and panic/Goexit privacy with default SDK panic recording enabled. Both direct SDK construction and telemetry/otel are exercised. No collector/exporter/socket is required. The span-only SDK benchmark is a reference cost, not equivalent to the full wrapper+histogram fixture; do not present their difference as pure wrapper overhead.
