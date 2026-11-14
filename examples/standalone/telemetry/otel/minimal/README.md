# Independent telemetry consumer

Local replace ../../../../../telemetry/otel is development-only. GOWORK=off GOPROXY=off GOFLAGS=-mod=readonly go run . and go test -race ./... use native SDK capture and ManualReader, without globals/exporter/network/framework. Its measured graph contains telemetry/otel, OTel API/SDK and legitimate transitives, not client.
