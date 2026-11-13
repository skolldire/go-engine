package circuit_breaker

import (
	"context"
	"testing"
	"time"
)

var boundaryResult any
var boundaryError error

func BenchmarkBoundarySuccess(b *testing.B) {
	breaker := NewCircuitBreaker(Dependencies{Config: &Config{Name: "bench", Interval: time.Hour}})
	ctx := context.Background()
	operation := func() (any, error) { return 42, nil }
	b.ReportAllocs()
	for b.Loop() {
		boundaryResult, boundaryError = breaker.Execute(ctx, operation)
	}
}
