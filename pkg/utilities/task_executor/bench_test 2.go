package task_executor

import (
	"context"
	"fmt"
	"testing"
)

// BenchmarkWorkerPool measures the pool's per-task overhead for trivial,
// CPU-free tasks, isolating the scheduling/result-collection cost.
func BenchmarkWorkerPool(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("tasks=%d", n), func(b *testing.B) {
			tasks := make(map[string]Tasker, n)
			for i := 0; i < n; i++ {
				tasks[fmt.Sprintf("t%d", i)] = &Task[int, int]{
					Func:     func(_ context.Context, x int) (int, error) { return x, nil },
					Args:     i,
					priority: PriorityNormal,
				}
			}

			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = WorkerPool(ctx, tasks, 8)
			}
		})
	}
}
