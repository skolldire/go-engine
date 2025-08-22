package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type Config struct {
	ServiceName    string  `json:"service_name"`
	ServiceVersion string  `json:"service_version"`
	Environment    string  `json:"environment"`
	OtelEndpoint   string  `json:"otel_endpoint"`
	SampleRate     float64 `json:"sample_rate"`
	Enabled        bool    `json:"enabled"`
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
	meter          metric.Meter
	tracer         oteltrace.Tracer
	attrs          []attribute.KeyValue
	traceProvider  *sdktrace.TracerProvider
	metricProvider *sdkmetric.MeterProvider
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
