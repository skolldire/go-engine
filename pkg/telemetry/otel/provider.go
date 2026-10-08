package otel

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

var _ Provider = (*realProvider)(nil)

type realProvider struct {
	traceProvider  *sdktrace.TracerProvider
	metricProvider *sdkmetric.MeterProvider

	// globals is the claim on the process-wide OpenTelemetry state, empty when
	// this provider did not install it. Shutdown returns it exactly once, and
	// only if it is still the holder.
	globals      globalClaim
	shutdownOnce sync.Once
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
	var claim globalClaim
	if !cfg.SkipGlobalProviders {
		var err error
		claim, err = claimGlobalProviders(cfg.ServiceName)
		if err != nil {
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
		globals:        claim,
	}, nil
}

func (p *realProvider) Tracer(name string) oteltrace.Tracer {
	return p.traceProvider.Tracer(name)
}

func (p *realProvider) Meter(name string) otelmetric.Meter {
	return p.metricProvider.Meter(name)
}

func (p *realProvider) Shutdown(ctx context.Context) error {
	// Give the claim back once, and only if we still hold it: a repeated
	// Shutdown must not free the globals a later provider has taken.
	p.shutdownOnce.Do(func() {
		// Point the globals at no-op providers before releasing the claim.
		// Releasing alone left them referencing the SDK that is about to be
		// shut down, so instrumentation kept recording into a dead pipeline
		// until some later provider happened to replace them — spans that go
		// nowhere and are never reported as lost.
		//
		// The propagator is deliberately left in place: it carries no
		// resources, and clearing it would break context propagation for code
		// that still runs during shutdown.
		if restoreGlobalProviders(p.globals) {
			otel.SetTracerProvider(tracenoop.NewTracerProvider())
			otel.SetMeterProvider(metricnoop.NewMeterProvider())
		}

		releaseGlobalProviders(p.globals)
	})

	// Shut down both providers even if the first fails, aggregating errors so a
	// trace-shutdown failure does not abandon the metric provider.
	// Guarded because the struct is constructible without them, and a Shutdown
	// that panics is worse than one that has nothing to do.
	var errs []error
	if p.traceProvider != nil {
		if err := p.traceProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown trace provider: %w", err))
		}
	}
	if p.metricProvider != nil {
		if err := p.metricProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown metric provider: %w", err))
		}
	}
	return errors.Join(errs...)
}
