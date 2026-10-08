package engine

import (
	"context"
	"sync/atomic"
)

// telemetrySwitch is the Telemetry handed to every provider. It is a stable
// indirection whose target can be replaced once, when a telemetry provider is
// built.
//
// It exists because Deps is assembled before any provider runs, while the real
// implementation only becomes available after the telemetry provider's Init.
// Handing providers the concrete implementation directly would mean whichever
// provider was built first captured the no-op forever — the same
// "implemented but never subscribed" defect the audit found in the config
// reload hooks.
type telemetrySwitch struct {
	target atomic.Value // Telemetry
}

var _ Telemetry = (*telemetrySwitch)(nil)

func newTelemetrySwitch(initial Telemetry) *telemetrySwitch {
	s := &telemetrySwitch{}
	s.set(initial)
	return s
}

func (s *telemetrySwitch) set(t Telemetry) {
	if t == nil {
		t = noopTelemetry{}
	}
	s.target.Store(&t)
}

func (s *telemetrySwitch) current() Telemetry {
	v := s.target.Load()
	if v == nil {
		return noopTelemetry{}
	}
	return *(v.(*Telemetry))
}

func (s *telemetrySwitch) Span(ctx context.Context, name string, fn func(context.Context) error) error {
	return s.current().Span(ctx, name, fn)
}

func (s *telemetrySwitch) Counter(ctx context.Context, name string, value int64) {
	s.current().Counter(ctx, name, value)
}

func (s *telemetrySwitch) Histogram(ctx context.Context, name string, value float64) {
	s.current().Histogram(ctx, name, value)
}

// TelemetrySource is implemented by a provider that can supply the engine's
// telemetry implementation. The core detects it after the provider is built and
// installs the result, so providers built before and after both record through
// it.
type TelemetrySource interface {
	Telemetry() Telemetry
}
