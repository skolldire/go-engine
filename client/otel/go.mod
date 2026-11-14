module github.com/skolldire/go-engine/client/otel

go 1.27.4

require (
	github.com/skolldire/go-engine/client v0.0.0
	go.opentelemetry.io/otel v1.47.0
	go.opentelemetry.io/otel/metric v1.47.0
	go.opentelemetry.io/otel/trace v1.47.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
)

replace github.com/skolldire/go-engine/client => ..
