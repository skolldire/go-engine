package concurrency_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	c "github.com/skolldire/go-engine/concurrency"
)

func TestIntegrationPipelineBackpressure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		input, err := c.From(ctx, 1, 2, 3, 4, 5, 6)
		if err != nil {
			t.Fatal(err)
		}
		mapped, err := c.Map(ctx, input, c.StreamConfig{Workers: 2, Buffer: 1}, func(_ context.Context, n int) (int, error) { return n * 2, nil })
		if err != nil {
			t.Fatal(err)
		}
		batches, err := c.Batch(ctx, mapped, c.BatchConfig{Size: 2, MaxBatchWeight: 2}, func(c.StreamResult[int]) int64 { return 1 })
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		seen := map[int]bool{}
		for batch := range batches {
			if batch.Err != nil {
				t.Fatal(batch.Err)
			}
			for _, r := range batch.Value {
				if r.Err != nil || seen[r.Value] {
					t.Fatalf("bad item %+v", r)
				}
				seen[r.Value] = true
			}
		}
		if len(seen) != 6 {
			t.Fatal(len(seen))
		}
	})
}

func TestIntegrationPoolsRetryAndSharedLimiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		limiter, err := c.NewLimiter(c.LimiterConfig{Capacity: 1})
		if err != nil {
			t.Fatal(err)
		}
		var ioActive, maxIO atomic.Int32
		var mu sync.Mutex
		order := map[string][]int{}
		attempts := map[string]int{}
		type item struct {
			key    string
			number int
		}
		task := func(ctx context.Context, in item) (int, error) {
			for attempt := 0; attempt < 2; attempt++ {
				release, err := limiter.Acquire(ctx, 1)
				if err != nil {
					return 0, err
				}
				active := ioActive.Add(1)
				for old := maxIO.Load(); active > old; old = maxIO.Load() {
					if maxIO.CompareAndSwap(old, active) {
						break
					}
				}
				mu.Lock()
				attempts[fmt.Sprint(in)]++
				mu.Unlock()
				synctest.Sleep(time.Millisecond)
				ioActive.Add(-1)
				release()
				if attempt == 0 {
					continue
				}
				mu.Lock()
				order[in.key] = append(order[in.key], in.number)
				mu.Unlock()
				return in.number, nil
			}
			panic("unreachable")
		}
		cfg := c.DefaultPoolConfig()
		cfg.Workers = 2
		cfg.QueueCapacity = 8
		cfg.MaxInFlightWeight = 10
		cfg.Ordering = c.OrderingPerKey
		cfg.MaxActiveKeys = 2
		cfg.PerKeyCapacity = 4
		cfg.KeyFailure = c.KeyFailureContinue
		pools := make([]*c.Pool[item, int], 2)
		var futures []*c.Future[int]
		for pi := range pools {
			p, err := c.NewPool(cfg, task)
			if err != nil {
				t.Fatal(err)
			}
			pools[pi] = p
			for n := range 3 {
				key := fmt.Sprint(pi)
				f, err := p.Submit(context.Background(), c.Job[item]{ID: fmt.Sprint(n), Key: key, Weight: 1, Value: item{key, n}})
				if err != nil {
					t.Fatal(err)
				}
				futures = append(futures, f)
			}
		}
		for _, f := range futures {
			r, err := f.Wait(context.Background())
			if err != nil || r.Err != nil {
				t.Fatalf("outcome %+v %v", r, err)
			}
		}
		for _, p := range pools {
			if err := p.Shutdown(context.Background(), c.ShutdownDrain); err != nil {
				t.Fatal(err)
			}
		}
		for _, numbers := range order {
			if fmt.Sprint(numbers) != "[0 1 2]" {
				t.Fatal(order)
			}
		}
		if maxIO.Load() != 1 {
			t.Fatalf("shared I/O bound %d", maxIO.Load())
		}
		for _, count := range attempts {
			if count != 2 {
				t.Fatal(attempts)
			}
		}
	})
}

func TestIntegrationPipelineCancelAndAbandonedFutures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		input, _ := c.From(ctx, 1, 2, 3, 4)
		mapped, _ := c.Map(ctx, input, c.StreamConfig{Workers: 2}, func(ctx context.Context, n int) (int, error) { return n, nil })
		batches, _ := c.Batch(ctx, mapped, c.BatchConfig{Size: 2, MaxBatchWeight: 2}, func(c.StreamResult[int]) int64 { return 1 })
		synctest.Wait()
		cancel()
		for range batches {
		}
		cfg := c.DefaultPoolConfig()
		cfg.Workers = 2
		cfg.QueueCapacity = 2
		cfg.MaxInFlightWeight = 4
		p, err := c.NewPool(cfg, func(context.Context, int) (int, error) { return 1, nil })
		if err != nil {
			t.Fatal(err)
		}
		for i := range 4 {
			if _, err := p.Submit(context.Background(), c.Job[int]{ID: fmt.Sprint(i), Weight: 1}); err != nil {
				t.Fatal(err)
			}
		}
		if err := p.Shutdown(context.Background(), c.ShutdownDrain); err != nil {
			t.Fatal(err)
		}
		if p.Stats().Succeeded != 4 {
			t.Fatal(p.Stats())
		}
	})
}
