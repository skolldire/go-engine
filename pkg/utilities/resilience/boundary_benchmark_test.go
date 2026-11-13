package resilience

import (
	"context"
	"testing"
)

var boundaryValue any
var boundaryError error

func BenchmarkBoundarySuccess(b *testing.B) {
	service := NewResilienceService(Config{}, nil)
	ctx := context.Background()
	operation := func(context.Context) (any, error) { return 42, nil }
	b.ReportAllocs()
	for b.Loop() {
		boundaryValue, boundaryError = service.Execute(ctx, operation)
	}
}
