package concurrency

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkMap(b *testing.B) {
	for _, workers := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			values := make([]int, 1024)
			for i := range values {
				values[i] = i
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				ctx, cancel := context.WithCancel(context.Background())
				out, err := Map(ctx, From(ctx, values...), Config{Workers: workers, Buffer: workers},
					func(_ context.Context, n int) (int, error) { return n * n, nil })
				if err != nil {
					cancel()
					b.Fatal(err)
				}
				for range out {
				}
				cancel()
			}
		})
	}
}
