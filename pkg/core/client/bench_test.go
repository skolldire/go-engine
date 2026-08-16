package client

import (
	"context"
	"testing"

	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
)

// BenchmarkBaseClientExecute measures the wrapper overhead added by
// BaseClient.Execute around a no-op operation, with and without the resilience
// layer enabled.
func BenchmarkBaseClientExecute(b *testing.B) {
	ctx := context.Background()
	op := func(context.Context) (any, error) { return nil, nil }

	b.Run("direct", func(b *testing.B) {
		bc := NewBaseClient(BaseConfig{Timeout: 5_000_000_000}, &mockLogger{})
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = bc.Execute(ctx, "op", op)
		}
	})

	b.Run("with_resilience", func(b *testing.B) {
		bc := NewBaseClient(BaseConfig{
			Timeout:        5_000_000_000,
			WithResilience: true,
			Resilience: resilience.Config{
				RetryConfig:          &retry_backoff.Config{MaxRetries: 3},
				CircuitBreakerConfig: &circuit_breaker.Config{Name: "bench"},
			},
		}, &mockLogger{})
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = bc.Execute(ctx, "op", op)
		}
	})
}
