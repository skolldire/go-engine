package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func NewTelemetry(ctx context.Context, config Config) (Telemetry, error) {
	if !config.Enabled {
		return &noopTelemetry{}, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(config.ServiceName),
			semconv.ServiceVersionKey.String(config.ServiceVersion),
			attribute.String("environment", config.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(config.OtelEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SampleRate)),
	)

	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(config.OtelEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetMeterProvider(metricProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	tel := &telemetry{
		meter:          metricProvider.Meter(config.ServiceName),
		tracer:         traceProvider.Tracer(config.ServiceName),
		traceProvider:  traceProvider,
		metricProvider: metricProvider,
		attrs: []attribute.KeyValue{
			attribute.String("service", config.ServiceName),
			attribute.String("environment", config.Environment),
		},
	}

	return tel, nil
}

func (t *telemetry) Counter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue) {
	counter, _ := t.meter.Int64Counter(name)
	counter.Add(ctx, value, metric.WithAttributes(append(t.attrs, attrs...)...))
}

func (t *telemetry) Gauge(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) {
	gauge, _ := t.meter.Float64Gauge(name)
	gauge.Record(ctx, value, metric.WithAttributes(append(t.attrs, attrs...)...))
}

func (t *telemetry) Histogram(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) {
	histogram, _ := t.meter.Float64Histogram(name)
	histogram.Record(ctx, value, metric.WithAttributes(append(t.attrs, attrs...)...))
}

func (t *telemetry) Span(ctx context.Context, name string, fn func(ctx context.Context) error, attrs ...attribute.KeyValue) error {
	ctx, span := t.tracer.Start(ctx, name, oteltrace.WithAttributes(append(t.attrs, attrs...)...))
	defer span.End()

	if err := fn(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

func (t *telemetry) Shutdown(ctx context.Context) error {
	if t.traceProvider != nil {
		if err := t.traceProvider.Shutdown(ctx); err != nil {
			return err
		}
	}
	if t.metricProvider != nil {
		if err := t.metricProvider.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (o *Operation) Execute(ctx context.Context, fn func(ctx context.Context) error, attrs ...attribute.KeyValue) error {
	start := time.Now()

	return o.tel.Span(ctx, o.name, func(ctx context.Context) error {
		err := fn(ctx)
		duration := time.Since(start).Seconds()

		o.tel.Counter(ctx, o.name+"_total", 1, attrs...)
		o.tel.Histogram(ctx, o.name+"_duration", duration, attrs...)

		if err != nil {
			o.tel.Counter(ctx, o.name+"_errors", 1, attrs...)
		}

		return err
	}, attrs...)
}

func NewOperation(tel Telemetry, name string) *Operation {
	return &Operation{tel: tel, name: name}
}
