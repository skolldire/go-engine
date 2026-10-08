package engine

import "context"

// The core hands providers non-nil collaborators even when the application
// configured none, so provider code never needs a nil check before timing an
// operation or registering a health check. The logger has no no-op form: New
// always builds a real one from the `log:` section.

type noopTelemetry struct{}

func (noopTelemetry) Span(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}
func (noopTelemetry) Counter(context.Context, string, int64)     {}
func (noopTelemetry) Histogram(context.Context, string, float64) {}

type noopHealth struct{}

func (noopHealth) RegisterCheck(string, func(ctx context.Context) error) {}
