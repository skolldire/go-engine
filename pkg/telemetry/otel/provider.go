package otel

import (
	"context"
	"errors"
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

	// ownsGlobals records whether this provider installed the process-wide
	// OpenTelemetry state, so Shutdown knows whether to release the claim.
	ownsGlobals bool
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

	traceOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.ExporterEndpoint)}
	if cfg.Insecure {
		traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		traceOpts = append(traceOpts, otlptracegrpc.WithHeaders(cfg.Headers))
	}

	traceExporter, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SamplingRate)),
	)

	metricOpts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.ExporterEndpoint)}
	if cfg.Insecure {
		metricOpts = append(metricOpts, otlpmetricgrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		metricOpts = append(metricOpts, otlpmetricgrpc.WithHeaders(cfg.Headers))
	}

	metricExporter, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		// Tear down the already-created trace provider so its exporter/goroutines
		// are not leaked when the metric exporter fails.
		_ = traceProvider.Shutdown(ctx)
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	// Registering globally is what lets third-party instrumentation emit through
	// this provider, so it is the default. It is process-wide, though: a second
	// engine in the same binary would overwrite the first and its spans would
	// disappear silently, which is why it can be skipped.
	if !cfg.SkipGlobalProviders {
		if err := claimGlobalProviders(cfg.ServiceName); err != nil {
			_ = traceProvider.Shutdown(ctx)
			_ = metricProvider.Shutdown(ctx)
			return nil, err
		}

		otel.SetTracerProvider(traceProvider)
		otel.SetMeterProvider(metricProvider)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
	}

	return &realProvider{
		traceProvider:  traceProvider,
		metricProvider: metricProvider,
		ownsGlobals:    !cfg.SkipGlobalProviders,
	}, nil
}

func (p *realProvider) Tracer(name string) oteltrace.Tracer {
	return p.traceProvider.Tracer(name)
}

func (p *realProvider) Meter(name string) otelmetric.Meter {
	return p.metricProvider.Meter(name)
}

func (p *realProvider) Shutdown(ctx context.Context) error {
	// Give the process-wide claim back, so a provider that has been shut down
	// does not lock the globals for the rest of the process's life.
	if p.ownsGlobals {
		releaseGlobalProviders()
	}

	// Shut down both providers even if the first fails, aggregating errors so a
	// trace-shutdown failure does not abandon the metric provider.
	var errs []error
	if err := p.traceProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown trace provider: %w", err))
	}
	if err := p.metricProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown metric provider: %w", err))
	}
	return errors.Join(errs...)
}
