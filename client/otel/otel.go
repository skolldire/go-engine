package otel

import (
	"context"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/client"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	componentKey        = "go_engine.client.component"
	operationKey        = "go_engine.client.operation"
	successKey          = "go_engine.client.success"
	durationName        = "go_engine.client.operation.duration"
	durationDescription = "Duration of normally completed logical client operations."
)

func New[T any](cfg Config, deps Dependencies) (client.Middleware[T], error) {
	if err := validate(cfg, deps); err != nil {
		return nil, err
	}
	var tracer trace.Tracer
	if deps.TracerProvider != nil {
		tracer = deps.TracerProvider.Tracer(cfg.InstrumentationName)
	}
	var histogram metric.Float64Histogram
	if deps.MeterProvider != nil {
		var err error
		histogram, err = deps.MeterProvider.Meter(cfg.InstrumentationName).Float64Histogram(durationName, metric.WithUnit("s"), metric.WithDescription(durationDescription))
		if err != nil {
			return nil, fmt.Errorf("client/otel: create duration histogram: %w", err)
		}
	}
	return middleware[T](tracer, histogram, deps.IsFailure, time.Now), nil
}

func middleware[T any](tracer trace.Tracer, histogram metric.Float64Histogram, isFailure func(error) bool, now func() time.Time) client.Middleware[T] {
	return func(next client.Handler[T]) client.Handler[T] {
		if next == nil {
			return nil
		}
		return func(ctx context.Context, info client.Invocation, operation client.Operation[T]) (T, error) {
			attrs := [3]attribute.KeyValue{attribute.String(componentKey, info.Component), attribute.String(operationKey, info.Operation)}
			var span trace.Span
			ended := false
			if tracer != nil {
				ctx, span = tracer.Start(ctx, info.Component+"."+info.Operation, trace.WithSpanKind(trace.SpanKindInternal), trace.WithAttributes(attrs[:2]...))
				defer func() {
					if !ended {
						span.End()
					}
				}()
			}
			var start time.Time
			if histogram != nil {
				start = now()
			}
			result, err := next(ctx, info, operation)
			var seconds float64
			if histogram != nil {
				seconds = now().Sub(start).Seconds()
			}
			attrs[2] = attribute.Bool(successKey, err == nil)
			if span != nil {
				span.SetAttributes(attrs[2])
				if err != nil && isFailure != nil && isFailure(err) {
					span.SetStatus(codes.Error, "")
				}
				ended = true
				span.End()
			}
			if histogram != nil {
				histogram.Record(ctx, seconds, metric.WithAttributes(attrs[:]...))
			}
			return result, err
		}
	}
}
