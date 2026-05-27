package otel

import (
	"context"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	oteltrace "go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	otelmetric "go.opentelemetry.io/otel/metric"
)

// OTELConfig holds OpenTelemetry provider configuration.
type OTELConfig struct {
	ServiceName      string  `mapstructure:"service_name" json:"service_name"`
	ServiceVersion   string  `mapstructure:"service_version" json:"service_version"`
	ExporterEndpoint string  `mapstructure:"exporter_endpoint" json:"exporter_endpoint"`
	SamplingRate     float64 `mapstructure:"sampling_rate" json:"sampling_rate"`
	Enabled          bool    `mapstructure:"enabled" json:"enabled"`
}

// Provider exposes the TracerProvider and MeterProvider for the application.
type Provider interface {
	Tracer(name string) oteltrace.Tracer
	Meter(name string) otelmetric.Meter
	Shutdown(ctx context.Context) error
}

// noopProvider is returned when Enabled = false.
type noopProvider struct{}

func (n *noopProvider) Tracer(_ string) oteltrace.Tracer {
	return tracenoop.NewTracerProvider().Tracer("")
}

func (n *noopProvider) Meter(_ string) otelmetric.Meter {
	return metricnoop.NewMeterProvider().Meter("")
}

func (n *noopProvider) Shutdown(_ context.Context) error { return nil }
