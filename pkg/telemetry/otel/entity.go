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
	ServiceName    string `mapstructure:"service_name" json:"service_name"`
	ServiceVersion string `mapstructure:"service_version" json:"service_version"`
	// Environment labels every metric and span produced through NewTelemetry.
	Environment      string  `mapstructure:"environment" json:"environment"`
	ExporterEndpoint string  `mapstructure:"exporter_endpoint" json:"exporter_endpoint"`
	SamplingRate     float64 `mapstructure:"sampling_rate" json:"sampling_rate"`
	Enabled          bool    `mapstructure:"enabled" json:"enabled"`
	// Insecure sends OTLP over plaintext gRPC. It defaults to false, so the
	// exporter uses TLS unless explicitly opted out (e.g. a local collector).
	Insecure bool `mapstructure:"insecure" json:"insecure"`
	// Headers are added to every OTLP export request, e.g. an API key/token for
	// a hosted collector.
	Headers map[string]string `mapstructure:"headers" json:"headers"`

	// SkipGlobalProviders leaves the process-wide OpenTelemetry providers
	// alone.
	//
	// By default the provider registers itself globally, which is what most
	// applications want: instrumentation libraries emit through
	// otel.GetTracerProvider() and would otherwise be silently dropped.
	//
	// The registration is process-wide, though, so two engines in one binary
	// overwrite each other and the loser's spans vanish with no error. Set this
	// on every engine but the one that owns the global state.
	SkipGlobalProviders bool `mapstructure:"skip_global_providers" json:"skip_global_providers"`
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
