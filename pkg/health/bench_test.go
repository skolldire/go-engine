package health

import (
	"context"
	"fmt"
	"testing"
)

// benchChecker is a trivial always-healthy checker used to isolate the
// fan-out/aggregation cost of GetStatus from any real dependency latency.
type benchChecker struct{}

func (benchChecker) Check(context.Context) error { return nil }

// BenchmarkGetStatus measures the concurrent fan-out and result aggregation
// cost of GetStatus as the number of registered checkers grows.
func BenchmarkGetStatus(b *testing.B) {
	ctx := context.Background()
	for _, n := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("checkers=%d", n), func(b *testing.B) {
			svc := NewService(Config{}, &mockLogger{})
			for i := 0; i < n; i++ {
				svc.Register(fmt.Sprintf("dep-%d", i), benchChecker{})
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = svc.GetStatus(ctx)
			}
		})
	}
}
