package telemetry

import (
	"context"
	"sync"

	otelprovider "github.com/skolldire/go-engine/pkg/telemetry/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type Config struct {
	// Every field carries a mapstructure tag. Without them the decoder fell back
	// to case-insensitive field-name matching, so YAML keys such as
	// `service_name` and `otel_endpoint` never bound: telemetry came up with an
	// empty endpoint and a zero sample rate while reporting itself as enabled.
	ServiceName    string  `mapstructure:"service_name" json:"service_name"`
	ServiceVersion string  `mapstructure:"service_version" json:"service_version"`
	Environment    string  `mapstructure:"environment" json:"environment"`
	OtelEndpoint   string  `mapstructure:"otel_endpoint" json:"otel_endpoint"`
	SampleRate     float64 `mapstructure:"sample_rate" json:"sample_rate"`
	Enabled        bool    `mapstructure:"enabled" json:"enabled"`
	// Insecure sends OTLP over plaintext gRPC. It defaults to false, so the
	// exporter uses TLS. Older versions of this package always exported in
	// plaintext; set this to true to keep that behaviour with a local collector.
	Insecure bool `mapstructure:"insecure" json:"insecure"`
}

type Metrics interface {
	Counter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue)
	Gauge(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue)
	Histogram(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue)
}

type Tracer interface {
	Span(ctx context.Context, name string, fn func(ctx context.Context) error, attrs ...attribute.KeyValue) error
}

type Telemetry interface {
	Metrics
	Tracer
	Shutdown(ctx context.Context) error
}

type telemetry struct {
	provider   otelprovider.Provider
	meter      metric.Meter
	tracer     oteltrace.Tracer
	attrs      []attribute.KeyValue
	counters   sync.Map // map[string]metric.Int64Counter
	gauges     sync.Map // map[string]metric.Float64Gauge
	histograms sync.Map // map[string]metric.Float64Histogram
}

type Operation struct {
	tel  Telemetry
	name string
}

type noopTelemetry struct{}

func (n *noopTelemetry) Counter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue) {
	// No-op implementation: silently ignore all operations
	// This is intentional for disabled telemetry
}

func (n *noopTelemetry) Gauge(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) {
	// No-op implementation: silently ignore all operations
	// This is intentional for disabled telemetry
}

func (n *noopTelemetry) Histogram(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) {
	// No-op implementation: silently ignore all operations
	// This is intentional for disabled telemetry
}
func (n *noopTelemetry) Span(ctx context.Context, name string, fn func(ctx context.Context) error, attrs ...attribute.KeyValue) error {
	return fn(ctx)
}
func (n *noopTelemetry) Shutdown(context.Context) error { return nil }
