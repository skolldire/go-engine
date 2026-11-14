// Package observer is consumer-local instrumentation depending only on OTel API.
package observer

import (
	"context"
	"github.com/skolldire/go-engine/client"
	"go.opentelemetry.io/otel/trace"
)

func Middleware[T any](provider trace.TracerProvider) client.Middleware[T] {
	tracer := provider.Tracer("consumer.observer")
	return func(next client.Handler[T]) client.Handler[T] {
		return func(ctx context.Context, info client.Invocation, op client.Operation[T]) (T, error) {
			ctx, span := tracer.Start(ctx, info.Operation)
			defer span.End()
			return next(ctx, info, op)
		}
	}
}
