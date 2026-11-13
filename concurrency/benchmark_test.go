package concurrency

import (
	"context"
	"errors"
	"fmt"
	"runtime"
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
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				input, err := From(ctx, values...)
				if err != nil {
					b.Fatal(err)
				}
				out, err := Map(ctx, input, StreamConfig{Workers: workers, Buffer: workers},
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

func BenchmarkPool(b *testing.B) {
	for _, mode := range []string{"trivial", "cooperative_yield", "hot_key", "payload_256"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				cfg := DefaultPoolConfig()
				cfg.Workers = 4
				cfg.QueueCapacity = 32
				cfg.MaxInFlightWeight = 36
				if mode == "hot_key" {
					cfg.Ordering = OrderingPerKey
					cfg.MaxActiveKeys = 4
					cfg.PerKeyCapacity = 32
					cfg.KeyFailure = KeyFailureContinue
				}
				p, err := NewPool(cfg, func(_ context.Context, n int) ([]byte, error) {
					if mode == "cooperative_yield" {
						runtime.Gosched()
					}
					if mode == "payload_256" {
						return make([]byte, 256), nil
					}
					return nil, nil
				})
				if err != nil {
					b.Fatal(err)
				}
				for i := range 32 {
					key := ""
					if mode == "hot_key" {
						key = "hot"
					}
					if _, err := p.Submit(context.Background(), Job[int]{ID: fmt.Sprint(i), Key: key, Weight: 1, Value: i}); err != nil {
						b.Fatal(err)
					}
				}
				if err := p.Shutdown(context.Background(), ShutdownDrain); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func BenchmarkSlowStreamConsumer(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		input, err := From(ctx, 1, 2, 3, 4, 5, 6, 7, 8)
		if err != nil {
			b.Fatal(err)
		}
		out, err := Map(ctx, input, StreamConfig{Workers: 4}, func(_ context.Context, n int) (int, error) { return n, nil })
		if err != nil {
			b.Fatal(err)
		}
		for range out {
			runtime.Gosched()
		}
		cancel()
	}
}

func BenchmarkPoolSteadyState(b *testing.B) {
	for _, workers := range []int{1, 4, 16} {
		for _, keyed := range []bool{false, true} {
			b.Run(fmt.Sprintf("workers=%d/keyed=%t", workers, keyed), func(b *testing.B) {
				cfg := DefaultPoolConfig()
				cfg.Workers = workers
				cfg.QueueCapacity = workers * 4
				cfg.MaxInFlightWeight = int64(workers * 5)
				if keyed {
					cfg.Ordering = OrderingPerKey
					cfg.MaxActiveKeys = 64
					cfg.PerKeyCapacity = 4
					cfg.KeyFailure = KeyFailureContinue
				}
				p, err := NewPool(cfg, func(_ context.Context, n int) (int, error) { return n + 1, nil })
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() {
					if err := p.Shutdown(context.Background(), ShutdownDrain); err != nil {
						b.Error(err)
					}
				})
				ctx := context.Background()
				futures := make([]*Future[int], workers)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					for i := range workers {
						key := ""
						if keyed {
							key = fmt.Sprint(i % 64)
						}
						future, err := p.Submit(ctx, Job[int]{ID: fmt.Sprint(i), Key: key, Weight: 1, Value: i})
						if err != nil {
							b.Fatal(err)
						}
						futures[i] = future
					}
					for i, future := range futures {
						result, err := future.Wait(ctx)
						if err != nil || result.Err != nil || result.Value != i+1 {
							b.Fatalf("result: %+v %v", result, err)
						}
					}
				}
				b.ReportMetric(float64(b.N*workers)/b.Elapsed().Seconds(), "jobs/s")
			})
		}
	}
}

func BenchmarkPoolSaturatedReject(b *testing.B) {
	cfg := DefaultPoolConfig()
	cfg.Admission = AdmissionReject
	gate := make(chan struct{})
	started := make(chan struct{})
	p, err := NewPool(cfg, func(context.Context, int) (int, error) { close(started); <-gate; return 1, nil })
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		close(gate)
		if err := p.Shutdown(context.Background(), ShutdownDrain); err != nil {
			b.Error(err)
		}
	})
	if _, err := p.Submit(context.Background(), Job[int]{ID: "running", Weight: 1}); err != nil {
		b.Fatal(err)
	}
	<-started
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.Submit(context.Background(), Job[int]{ID: "rejected", Weight: 1}); !errors.Is(err, ErrSaturated) {
			b.Fatal(err)
		}
	}
}

func BenchmarkLimiterContended(b *testing.B) {
	for _, capacity := range []int64{1, 4, 16} {
		b.Run(fmt.Sprintf("capacity=%d", capacity), func(b *testing.B) {
			limiter, err := NewLimiter(LimiterConfig{Capacity: capacity})
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					release, err := limiter.Acquire(context.Background(), 1)
					if err != nil {
						b.Error(err)
						return
					}
					runtime.Gosched()
					release()
				}
			})
		})
	}
}

func BenchmarkBatchWeighted(b *testing.B) {
	values := make([]int, 1024)
	for i := range values {
		values[i] = 1 + i%3
	}
	for _, size := range []int{1, 16, 128} {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				input, err := From(ctx, values...)
				if err != nil {
					b.Fatal(err)
				}
				batches, err := Batch(ctx, input, BatchConfig{Size: size, Buffer: 1, MaxBatchWeight: 64}, func(n int) int64 { return int64(n) })
				if err != nil {
					cancel()
					b.Fatal(err)
				}
				count := 0
				for batch := range batches {
					if batch.Err != nil {
						cancel()
						b.Fatal(batch.Err)
					}
					count += len(batch.Value)
				}
				cancel()
				if count != len(values) {
					b.Fatal(count)
				}
			}
		})
	}
}

func BenchmarkPoolPendingKeys(b *testing.B) {
	for _, pending := range []int{16, 128, 1024} {
		b.Run(fmt.Sprintf("pending=%d", pending), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				gate := make(chan struct{})
				started := make(chan struct{}, 4)
				cfg := DefaultPoolConfig()
				cfg.Workers = 4
				cfg.QueueCapacity = pending
				cfg.MaxInFlightWeight = int64(pending + 4)
				cfg.Ordering = OrderingPerKey
				cfg.MaxActiveKeys = pending + 4
				cfg.PerKeyCapacity = 1
				cfg.KeyFailure = KeyFailureContinue
				p, err := NewPool(cfg, func(_ context.Context, n int) (int, error) {
					if n < 4 {
						started <- struct{}{}
					}
					<-gate
					return n, nil
				})
				if err != nil {
					b.Fatal(err)
				}
				for i := range pending + 4 {
					key := fmt.Sprint(i)
					if _, err := p.Submit(context.Background(), Job[int]{ID: key, Key: key, Weight: 1, Value: i}); err != nil {
						close(gate)
						p.Shutdown(context.Background(), ShutdownCancel)
						b.Fatal(err)
					}
					if i < 4 {
						<-started
					}
				}
				close(gate)
				if err := p.Shutdown(context.Background(), ShutdownDrain); err != nil {
					b.Fatal(err)
				}
				if p.Stats().Succeeded != uint64(pending+4) {
					b.Fatal(p.Stats())
				}
			}
			b.ReportMetric(float64(b.N*(pending+4))/b.Elapsed().Seconds(), "jobs/s")
		})
	}
}
