package client

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

// WithResilience wraps operations in the existing retry + circuit-breaker
// service.
//
// It is a Middleware like logging or metrics rather than an `if` branch inside
// Execute, which is what lets a client take resilience without logging, or
// place it relative to the other concerns rather than accepting a fixed order.
//
// The operation must be idempotent: the retry layer may run it more than once.
func WithResilience(cfg resilience.Config, log logger.Service) Middleware {
	svc := resilience.NewResilienceService(cfg, log)
	return WithResilienceService(svc)
}

// WithResilienceService is WithResilience for an already-built service, so
// several clients can share one circuit breaker over the same dependency.
func WithResilienceService(svc *resilience.Service) Middleware {
	return func(next Handler) Handler {
		if svc == nil {
			return next
		}
		return func(ctx context.Context, inv Invocation, op Operation) (any, error) {
			return svc.Execute(ctx, func(ctx context.Context) (any, error) {
				return next(ctx, inv, op)
			})
		}
	}
}
