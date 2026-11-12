package otel

import (
	"context"
	"log"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Metrics records instrument values without exposing the OTel meter, so a
// caller can be handed only the ability to record.
type Metrics interface {
	Counter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue)
	Gauge(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue)
	Histogram(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue)
}

// Tracer runs fn inside a span. It is deliberately narrower than
// oteltrace.Tracer: a caller cannot leak a span by forgetting to End it.
type Tracer interface {
	Span(ctx context.Context, name string, fn func(ctx context.Context) error, attrs ...attribute.KeyValue) error
}

// Telemetry is the recording surface handed to adapters that instrument
// themselves. Construct it with NewTelemetry.
type Telemetry interface {
	Metrics
	Tracer
	Shutdown(ctx context.Context) error
}

type telemetry struct {
	provider Provider
	meter    metric.Meter
	tracer   oteltrace.Tracer
	attrs    []attribute.KeyValue
	// Instruments are cached because the meter allocates a new one per call;
	// recording in a hot path would otherwise churn one object per data point.
	counters   sync.Map // map[string]metric.Int64Counter
	gauges     sync.Map // map[string]metric.Float64Gauge
	histograms sync.Map // map[string]metric.Float64Histogram
}

// NewTelemetry builds a Telemetry on top of a new Provider.
//
// It owns the provider it creates, so Shutdown tears the exporters down. Use
// NewTelemetryFrom when the provider is already owned by someone else — two
// providers mean two OTLP exporters and two sets of goroutines, with whichever
// registered last silently discarding the other's spans.
func NewTelemetry(ctx context.Context, cfg OTELConfig) (Telemetry, error) {
	if !cfg.Enabled {
		return &noopTelemetry{}, nil
	}

	provider, err := NewProvider(ctx, cfg)
	if err != nil {
		return nil, err
	}

	t := newTelemetryFrom(provider, cfg)
	t.provider = provider
	return t, nil
}

// NewTelemetryFrom wraps an existing Provider. Shutdown is a no-op: the caller
// that built the provider stays responsible for tearing it down.
func NewTelemetryFrom(provider Provider, cfg OTELConfig) Telemetry {
	if provider == nil {
		return &noopTelemetry{}
	}
	return newTelemetryFrom(provider, cfg)
}

func newTelemetryFrom(provider Provider, cfg OTELConfig) *telemetry {
	return &telemetry{
		meter:  provider.Meter(cfg.ServiceName),
		tracer: provider.Tracer(cfg.ServiceName),
		attrs: []attribute.KeyValue{
			attribute.String("service", cfg.ServiceName),
			attribute.String("environment", cfg.Environment),
		},
	}
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

type noopTelemetry struct{}

// The no-op implementation exists so a disabled configuration needs no nil
// checks at every call site.
func (n *noopTelemetry) Counter(context.Context, string, int64, ...attribute.KeyValue)     {}
func (n *noopTelemetry) Gauge(context.Context, string, float64, ...attribute.KeyValue)     {}
func (n *noopTelemetry) Histogram(context.Context, string, float64, ...attribute.KeyValue) {}
func (n *noopTelemetry) Span(ctx context.Context, _ string, fn func(ctx context.Context) error, _ ...attribute.KeyValue) error {
	return fn(ctx)
}
func (n *noopTelemetry) Shutdown(context.Context) error { return nil }

// Operation instruments a named unit of work with the three signals it almost
// always needs together: a span, a counter, and a duration histogram.
type Operation struct {
	tel  Telemetry
	name string
}

// NewOperation returns an Operation recording under name.
func NewOperation(tel Telemetry, name string) *Operation {
	return &Operation{tel: tel, name: name}
}

// Execute runs fn inside the operation's span and records its outcome.
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
