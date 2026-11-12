package client

import (
	"context"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// Recorder receives one measurement per operation.
//
// It is deliberately narrower than the AWS-shaped MetricsRecorder in
// pkg/integration/observability: this one must work for a Redis GET and a Kafka
// publish as well as an HTTP call, so it carries no status code or HTTP verb.
type Recorder interface {
	RecordOperation(ctx context.Context, inv Invocation, duration time.Duration, err error)
}

// Tracer starts a span around an operation. The signature matches the engine's
// telemetry interface, so the OpenTelemetry provider satisfies it directly and
// the core needs no dependency on the OTel SDK.
type Tracer interface {
	Span(ctx context.Context, name string, fn func(context.Context) error) error
}

// WithLogging logs the start and outcome of every operation.
//
// enabled is consulted per call so SetLogging keeps working at runtime. It may
// be nil, meaning always enabled.
func WithLogging(log logger.Service, enabled func() bool) Middleware {
	return func(next Handler) Handler {
		if log == nil {
			return next
		}
		return func(ctx context.Context, inv Invocation, op Operation) (any, error) {
			if enabled != nil && !enabled() {
				return next(ctx, inv, op)
			}

			fields := map[string]any{
				"operation": inv.Operation,
				"service":   inv.Client,
			}

			log.Debug(ctx, "starting operation: "+inv.Operation, fields)
			result, err := next(ctx, inv, op)
			if err != nil {
				log.Error(ctx, err, fields)
			} else {
				log.Debug(ctx, "operation completed: "+inv.Operation, fields)
			}
			return result, err
		}
	}
}

// WithMetrics reports the duration and outcome of every operation.
func WithMetrics(rec Recorder) Middleware {
	return func(next Handler) Handler {
		if rec == nil {
			return next
		}
		return func(ctx context.Context, inv Invocation, op Operation) (any, error) {
			start := time.Now()
			result, err := next(ctx, inv, op)
			rec.RecordOperation(ctx, inv, time.Since(start), err)
			return result, err
		}
	}
}

// WithTracing wraps every operation in a span named "<client>.<operation>".
func WithTracing(tracer Tracer) Middleware {
	return func(next Handler) Handler {
		if tracer == nil {
			return next
		}
		return func(ctx context.Context, inv Invocation, op Operation) (any, error) {
			var result any
			err := tracer.Span(ctx, inv.Client+"."+inv.Operation, func(ctx context.Context) error {
				var opErr error
				result, opErr = next(ctx, inv, op)
				return opErr
			})
			return result, err
		}
	}
}
