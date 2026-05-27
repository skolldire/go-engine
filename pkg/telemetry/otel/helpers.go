package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// SpanFromContext returns the current span stored in ctx.
// Returns a no-op span when none exists — always safe to call.
func SpanFromContext(ctx context.Context) oteltrace.Span {
	return oteltrace.SpanFromContext(ctx)
}

// AddSpanEvent attaches a named event with optional attributes to the span in ctx.
func AddSpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	oteltrace.SpanFromContext(ctx).AddEvent(name, oteltrace.WithAttributes(attrs...))
}

// RecordError marks the span in ctx as failed and records err.
// No-op when err is nil.
func RecordError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	span := oteltrace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
