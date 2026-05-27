package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var _ Provider = (*realProvider)(nil)

type realProvider struct {
	traceProvider  *sdktrace.TracerProvider
	metricProvider *sdkmetric.MeterProvider
}

// NewProvider initializes a new OTel Provider from cfg.
// Returns a no-op Provider when cfg.Enabled is false — safe to use without any OTLP backend.
func NewProvider(ctx context.Context, cfg OTELConfig) (Provider, error) {
	if !cfg.Enabled {
		return &noopProvider{}, nil
	}

	if cfg.SamplingRate <= 0 {
		cfg.SamplingRate = 1.0
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.ExporterEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SamplingRate)),
	)

	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.ExporterEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetMeterProvider(metricProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &realProvider{
		traceProvider:  traceProvider,
		metricProvider: metricProvider,
	}, nil
}

func (p *realProvider) Tracer(name string) oteltrace.Tracer {
	return p.traceProvider.Tracer(name)
}

func (p *realProvider) Meter(name string) otelmetric.Meter {
	return p.metricProvider.Meter(name)
}

func (p *realProvider) Shutdown(ctx context.Context) error {
	if err := p.traceProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown trace provider: %w", err)
	}
	if err := p.metricProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown metric provider: %w", err)
	}
	return nil
}
