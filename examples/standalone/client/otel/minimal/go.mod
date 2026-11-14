module example.com/client-otel-minimal

go 1.27.4

require (
	github.com/skolldire/go-engine/client v0.0.0
	github.com/skolldire/go-engine/client/otel v0.0.0
	go.opentelemetry.io/otel/metric v1.47.0
	go.opentelemetry.io/otel/trace v1.47.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
)

replace github.com/skolldire/go-engine/client => ../../../../../client

replace github.com/skolldire/go-engine/client/otel => ../../../../../client/otel
