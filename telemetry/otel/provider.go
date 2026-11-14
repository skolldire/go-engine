package otel

import (
	"context"
	"errors"
	"math"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrInvalidConfig = errors.New("invalid telemetry configuration")
	ErrNilContext    = errors.New("telemetry: nil context")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "telemetry/otel: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	ServiceName             string  `mapstructure:"service_name" json:"service_name"`
	ServiceVersion          string  `mapstructure:"service_version" json:"service_version"`
	Environment             string  `mapstructure:"environment" json:"environment"`
	SamplingRatio           float64 `mapstructure:"sampling_ratio" json:"sampling_ratio"`
	MaxAttributes           int     `mapstructure:"max_attributes" json:"max_attributes"`
	MaxAttributeValueLength int     `mapstructure:"max_attribute_value_length" json:"max_attribute_value_length"`
}
type Dependencies struct {
	TraceProcessor sdktrace.SpanProcessor
	MetricReader   sdkmetric.Reader
}
type Provider struct {
	tracing *sdktrace.TracerProvider
	metrics *sdkmetric.MeterProvider
	mu      sync.Mutex
	started bool
	done    chan struct{}
	result  error
}

func DefaultConfig() Config {
	return Config{SamplingRatio: 1, MaxAttributes: 128, MaxAttributeValueLength: 4096}
}
func (cfg Config) Validate() error {
	switch {
	case cfg.ServiceName == "":
		return &ConfigError{Field: "ServiceName", Reason: "is required"}
	case math.IsNaN(cfg.SamplingRatio) || math.IsInf(cfg.SamplingRatio, 0) || cfg.SamplingRatio < 0 || cfg.SamplingRatio > 1:
		return &ConfigError{Field: "SamplingRatio", Reason: "must be finite and in [0,1]"}
	case cfg.MaxAttributes <= 0:
		return &ConfigError{Field: "MaxAttributes", Reason: "must be positive"}
	case cfg.MaxAttributeValueLength <= 0:
		return &ConfigError{Field: "MaxAttributeValueLength", Reason: "must be positive"}
	}
	return nil
}
func New(cfg Config, deps Dependencies) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	attrs := []attribute.KeyValue{semconv.ServiceName(cfg.ServiceName)}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	if cfg.Environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentNameKey.String(cfg.Environment))
	}
	res := resource.NewWithAttributes(semconv.SchemaURL, attrs...)
	traceOptions := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SamplingRatio))),
		sdktrace.WithRawSpanLimits(sdktrace.SpanLimits{
			AttributeValueLengthLimit:   cfg.MaxAttributeValueLength,
			AttributeValueDepthLimit:    sdktrace.DefaultAttributeValueDepthLimit,
			AttributeCountLimit:         cfg.MaxAttributes,
			AttributePerEventCountLimit: cfg.MaxAttributes,
			AttributePerLinkCountLimit:  cfg.MaxAttributes,
			EventCountLimit:             128, LinkCountLimit: 128,
		}),
	}
	if deps.TraceProcessor != nil {
		traceOptions = append(traceOptions, sdktrace.WithSpanProcessor(deps.TraceProcessor))
	}
	metricOptions := []sdkmetric.Option{sdkmetric.WithResource(res), sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter), sdkmetric.WithCardinalityLimit(0)}
	if deps.MetricReader != nil {
		metricOptions = append(metricOptions, sdkmetric.WithReader(deps.MetricReader))
	}
	return &Provider{tracing: sdktrace.NewTracerProvider(traceOptions...), metrics: sdkmetric.NewMeterProvider(metricOptions...), done: make(chan struct{})}, nil
}
func (p *Provider) Tracer(name string) trace.Tracer      { return p.tracing.Tracer(name) }
func (p *Provider) Meter(name string) metric.Meter       { return p.metrics.Meter(name) }
func (p *Provider) TracerProvider() trace.TracerProvider { return p.tracing }
func (p *Provider) MeterProvider() metric.MeterProvider  { return p.metrics }
func (p *Provider) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		select {
		case <-p.done:
			return p.result
		default:
		}
		select {
		case <-p.done:
			return p.result
		case <-ctx.Done():
			return errors.Join(ctx.Err(), context.Cause(ctx))
		}
	}
	p.started = true
	p.mu.Unlock()
	return p.close(ctx)
}
func (p *Provider) close(ctx context.Context) (result error) {
	completed := false
	var traceError, metricError error
	defer func() {
		if !completed {
			result = errors.Join(errors.New("telemetry shutdown did not return normally"), traceError, metricError)
		}
		p.mu.Lock()
		p.result = result
		close(p.done)
		p.mu.Unlock()
	}()
	func() { defer func() { metricError = p.metrics.Shutdown(ctx) }(); traceError = p.tracing.Shutdown(ctx) }()
	completed = true
	return errors.Join(traceError, metricError)
}
