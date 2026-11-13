package retry_backoff

import (
	"context"
	"testing"
)

var boundaryValue int
var boundaryError error

func BenchmarkBoundarySuccess(b *testing.B) {
	retryer := NewRetryer(Dependencies{})
	ctx := context.Background()
	operation := func() error { boundaryValue = 42; return nil }
	b.ReportAllocs()
	for b.Loop() {
		boundaryError = retryer.Do(ctx, operation)
	}
}
