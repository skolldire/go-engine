// Package otel supplies an OpenTelemetry implementation of engine.Telemetry.
//
// It is a separate package on purpose: the core declares only the narrow
// engine.Telemetry interface, so an application without telemetry never links
// the OTel SDK, its OTLP exporters or their gRPC transport.
package otel

import (
	"context"
	"fmt"

	"github.com/skolldire/go-engine/pkg/engine"
	pkgotel "github.com/skolldire/go-engine/pkg/telemetry/otel"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "telemetry"

// Provider builds the OpenTelemetry provider and exposes it as the engine's
// telemetry implementation.
type Provider struct {
	serviceName string
	provider    pkgotel.Provider
	telemetry   *telemetry
}

var _ engine.Provider = (*Provider)(nil)

// New returns the telemetry provider. The service name defaults to the one in
// the configuration file when left empty.
func New() *Provider { return &Provider{} }

// Name implements engine.Provider.
func (p *Provider) Name() string { return "telemetry" }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, _ engine.Deps) (any, error) {
	var cfg pkgotel.OTELConfig
	if raw.Exists() {
		if err := checkLegacyKeys(raw); err != nil {
			return nil, err
		}
		if err := raw.Decode(&cfg); err != nil {
			return nil, err
		}
	}

	provider, err := pkgotel.NewProvider(ctx, cfg)
	if err != nil {
		return nil, err
	}

	p.serviceName = cfg.ServiceName
	if p.serviceName == "" {
		p.serviceName = "go-engine"
	}
	p.provider = provider
	p.telemetry = &telemetry{
		tracer: provider.Tracer(p.serviceName),
		meter:  provider.Meter(p.serviceName),
	}

	return provider, nil
}

// legacyKeys maps the key names used by the deprecated
// pkg/utilities/telemetry configuration to their replacements.
var legacyKeys = map[string]string{
	"otel_endpoint": "exporter_endpoint",
	"sample_rate":   "sampling_rate",
}

// checkLegacyKeys rejects a telemetry section written for the old package.
// Decoding it silently would leave the endpoint empty and the sampling rate at
// zero while reporting telemetry as enabled, so the traces would vanish with no
// indication of why.
func checkLegacyKeys(raw engine.RawConfig) error {
	section, ok := raw.Raw().(map[string]any)
	if !ok {
		return nil
	}
	for old, replacement := range legacyKeys {
		if _, present := section[old]; present {
			return fmt.Errorf(
				"telemetry: %q was renamed to %q; see docs/migration-engine.md", old, replacement)
		}
	}
	return nil
}

// Close implements engine.Provider.
func (p *Provider) Close(ctx context.Context) error {
	if p.provider == nil {
		return nil
	}
	return p.provider.Shutdown(ctx)
}

// Telemetry returns the engine.Telemetry view of this provider. Pass it to
// engine.WithTelemetry so every other provider records through it.
func (p *Provider) Telemetry() engine.Telemetry {
	if p.telemetry == nil {
		return nil
	}
	return p.telemetry
}

// From retrieves the OTel provider from an engine.
func From(e *engine.Engine) (pkgotel.Provider, error) {
	return engine.Get[pkgotel.Provider](e, "telemetry")
}

// telemetry adapts the OTel tracer and meter onto the core's narrow interface.
// This package is allowed to name OTel types; the core is not.
type telemetry struct {
	tracer oteltrace.Tracer
	meter  otelmetric.Meter
}

func (t *telemetry) Span(ctx context.Context, name string, fn func(context.Context) error) error {
	ctx, s := t.tracer.Start(ctx, name)
	defer s.End()

	if err := fn(ctx); err != nil {
		s.RecordError(err)
		s.SetStatus(codes.Error, err.Error())
		return err
	}
	s.SetStatus(codes.Ok, "")
	return nil
}

func (t *telemetry) Counter(ctx context.Context, name string, value int64) {
	c, err := t.meter.Int64Counter(name)
	if err != nil {
		return
	}
	c.Add(ctx, value)
}

func (t *telemetry) Histogram(ctx context.Context, name string, value float64) {
	h, err := t.meter.Float64Histogram(name)
	if err != nil {
		return
	}
	h.Record(ctx, value)
}
