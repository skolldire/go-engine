package concurrency

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"testing"
)

func TestStreamingConfigurationMatrix(t *testing.T) {
	for _, workers := range []int{1, 4, 16} {
		for _, buffer := range []int{0, 1, 16} {
			for _, size := range []int{1, 3, 32} {
				t.Run(fmt.Sprintf("w%d/b%d/size%d", workers, buffer, size), func(t *testing.T) {
					ctx, cancel := context.WithCancel(testContext(t))
					defer cancel()
					values := make([]int, 127)
					for i := range values {
						values[i] = i
					}
					input, err := From(ctx, values...)
					if err != nil {
						t.Fatal(err)
					}
					var active atomic.Int64
					mapped, err := Map(ctx, input, StreamConfig{Workers: workers, Buffer: buffer}, func(_ context.Context, n int) (int, error) {
						if active.Add(1) > int64(workers) {
							t.Error("map worker limit exceeded")
						}
						active.Add(-1)
						return n, nil
					})
					if err != nil {
						t.Fatal(err)
					}
					branches, err := FanOut(ctx, mapped, 3, buffer)
					if err != nil {
						t.Fatal(err)
					}
					merged, err := FanIn(ctx, buffer, branches...)
					if err != nil {
						t.Fatal(err)
					}
					batches, err := Batch(ctx, merged, BatchConfig{Size: size, Buffer: buffer, MaxBatchWeight: 7}, func(r StreamResult[int]) int64 { return int64(1 + r.Value%3) })
					if err != nil {
						t.Fatal(err)
					}
					seen := map[int]bool{}
					for batch := range batches {
						if batch.Err != nil || len(batch.Value) == 0 || len(batch.Value) > size {
							t.Fatalf("batch: %+v", batch)
						}
						weight := 0
						for _, r := range batch.Value {
							if r.Err != nil || seen[r.Value] || r.Value < 0 || r.Value >= len(values) {
								t.Fatalf("result: %+v", r)
							}
							seen[r.Value] = true
							weight += 1 + r.Value%3
						}
						if weight > 7 {
							t.Fatal("batch weight exceeded")
						}
					}
					if len(seen) != len(values) || active.Load() != 0 {
						t.Fatalf("conservation: %d/%d, active=%d", len(seen), len(values), active.Load())
					}
				})
			}
		}
	}
}

func TestWeightedArithmeticBoundaries(t *testing.T) {
	ctx := testContext(t)
	limiter, err := NewLimiter(LimiterConfig{Capacity: math.MaxInt64})
	if err != nil {
		t.Fatal(err)
	}
	release, err := limiter.Acquire(ctx, math.MaxInt64-1)
	if err != nil {
		t.Fatal(err)
	}
	last, err := limiter.Acquire(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	release()
	last()
	release()
	last()
	full, err := limiter.Acquire(ctx, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	full()
	input, err := From(ctx, int64(math.MaxInt64-1), int64(2), int64(math.MaxInt64))
	if err != nil {
		t.Fatal(err)
	}
	batches, err := Batch(ctx, input, BatchConfig{Size: 3, MaxBatchWeight: math.MaxInt64}, func(n int64) int64 { return n })
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for batch := range batches {
		if batch.Err != nil || len(batch.Value) != 1 {
			t.Fatalf("overflow: %+v", batch)
		}
		count++
	}
	if count != 3 {
		t.Fatal(count)
	}
}
