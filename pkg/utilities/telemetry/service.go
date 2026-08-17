package telemetry

import (
	"context"
	"log"
	"time"

	otelprovider "github.com/skolldire/go-engine/pkg/telemetry/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// NewTelemetry builds a Telemetry backed by pkg/telemetry/otel.
//
// Deprecated: use pkg/telemetry/otel.NewProvider directly. This package is a
// thin facade kept for compatibility. It used to own a second, independent
// OpenTelemetry setup that also called otel.SetTracerProvider/SetMeterProvider;
// an application configuring telemetry through YAML *and* calling WithOTEL got
// two providers, two OTLP exporters and two sets of goroutines, with whichever
// registered last silently discarding the other's spans.
func NewTelemetry(ctx context.Context, config Config) (Telemetry, error) {
	if !config.Enabled {
		return &noopTelemetry{}, nil
	}

	provider, err := otelprovider.NewProvider(ctx, otelprovider.OTELConfig{
		ServiceName:      config.ServiceName,
		ServiceVersion:   config.ServiceVersion,
		ExporterEndpoint: config.OtelEndpoint,
		SamplingRate:     config.SampleRate,
		Enabled:          true,
		Insecure:         config.Insecure,
	})
	if err != nil {
		return nil, err
	}

	return &telemetry{
		provider: provider,
		meter:    provider.Meter(config.ServiceName),
		tracer:   provider.Tracer(config.ServiceName),
		attrs: []attribute.KeyValue{
			attribute.String("service", config.ServiceName),
			attribute.String("environment", config.Environment),
		},
	}, nil
}

func (t *telemetry) Counter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue) {
	if v, ok := t.counters.Load(name); ok {
		v.(metric.Int64Counter).Add(ctx, value, metric.WithAttributes(append(t.attrs, attrs...)...))
		return
	}
	c, err := t.meter.Int64Counter(name)
	if err != nil {
		log.Printf("[telemetry] warning: failed to create counter %q: %v", name, err)
		return
	}
	actual, _ := t.counters.LoadOrStore(name, c)
	actual.(metric.Int64Counter).Add(ctx, value, metric.WithAttributes(append(t.attrs, attrs...)...))
}

func (t *telemetry) Gauge(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) {
	if v, ok := t.gauges.Load(name); ok {
		v.(metric.Float64Gauge).Record(ctx, value, metric.WithAttributes(append(t.attrs, attrs...)...))
		return
	}
	g, err := t.meter.Float64Gauge(name)
	if err != nil {
		log.Printf("[telemetry] warning: failed to create gauge %q: %v", name, err)
		return
	}
	actual, _ := t.gauges.LoadOrStore(name, g)
	actual.(metric.Float64Gauge).Record(ctx, value, metric.WithAttributes(append(t.attrs, attrs...)...))
}

func (t *telemetry) Histogram(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) {
	if v, ok := t.histograms.Load(name); ok {
		v.(metric.Float64Histogram).Record(ctx, value, metric.WithAttributes(append(t.attrs, attrs...)...))
		return
	}
	h, err := t.meter.Float64Histogram(name)
	if err != nil {
		log.Printf("[telemetry] warning: failed to create histogram %q: %v", name, err)
		return
	}
	actual, _ := t.histograms.LoadOrStore(name, h)
	actual.(metric.Float64Histogram).Record(ctx, value, metric.WithAttributes(append(t.attrs, attrs...)...))
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
	if t.provider == nil {
		return nil
	}
	return t.provider.Shutdown(ctx)
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
