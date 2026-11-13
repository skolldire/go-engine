package main

import (
	"context"
	"fmt"
	c "github.com/skolldire/go-engine/concurrency"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type job struct {
	key      string
	sequence int
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("composed: pipeline, keyed retry, two pools and shared limiter OK")
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	limiter, err := c.NewLimiter(c.LimiterConfig{Capacity: 1})
	if err != nil {
		return err
	}
	var active, maxActive atomic.Int32
	var mu sync.Mutex
	order := map[string][]int{}
	task := func(ctx context.Context, in job) (int, error) {
		for attempt := 0; attempt < 2; attempt++ {
			release, err := limiter.Acquire(ctx, 1)
			if err != nil {
				return 0, err
			}
			current := active.Add(1)
			for old := maxActive.Load(); current > old; old = maxActive.Load() {
				if maxActive.CompareAndSwap(old, current) {
					break
				}
			}
			runtime.Gosched()
			active.Add(-1)
			release()
			if attempt == 0 {
				continue
			}
			mu.Lock()
			order[in.key] = append(order[in.key], in.sequence)
			mu.Unlock()
			return in.sequence, nil
		}
		return 0, fmt.Errorf("retry budget exhausted")
	}
	cfg := c.DefaultPoolConfig()
	cfg.Workers = 2
	cfg.QueueCapacity = 4
	cfg.MaxInFlightWeight = 6
	cfg.Ordering = c.OrderingPerKey
	cfg.MaxActiveKeys = 2
	cfg.PerKeyCapacity = 4
	cfg.KeyFailure = c.KeyFailureContinue
	var futures []*c.Future[int]
	var pools []*c.Pool[job, int]
	defer func() {
		for _, p := range pools {
			_ = p.Shutdown(ctx, c.ShutdownCancel)
		}
	}()
	for pi := range 2 {
		p, err := c.NewPool(cfg, task)
		if err != nil {
			return err
		}
		pools = append(pools, p)
		for i := range 4 {
			key := fmt.Sprint(pi)
			f, err := p.Submit(ctx, c.Job[job]{ID: fmt.Sprint(i), Key: key, Weight: 1, Value: job{key, i}})
			if err != nil {
				return err
			}
			futures = append(futures, f)
		}
	}
	for _, f := range futures {
		r, err := f.Wait(ctx)
		if err != nil {
			return err
		}
		if r.Err != nil {
			return r.Err
		}
	}
	for _, p := range pools {
		if err := p.Shutdown(ctx, c.ShutdownDrain); err != nil {
			return err
		}
	}
	if maxActive.Load() != 1 {
		return fmt.Errorf("external concurrency=%d", maxActive.Load())
	}
	for _, seq := range order {
		if fmt.Sprint(seq) != "[0 1 2 3]" {
			return fmt.Errorf("key order=%v", order)
		}
	}
	input, err := c.From(ctx, 1, 2, 3, 4)
	if err != nil {
		return err
	}
	mapped, err := c.Map(ctx, input, c.StreamConfig{Workers: 2, Buffer: 1}, func(_ context.Context, n int) (int, error) { return n * 2, nil })
	if err != nil {
		return err
	}
	batch, err := c.Batch(ctx, mapped, c.BatchConfig{Size: 2, Buffer: 1, MaxBatchWeight: 2}, func(c.StreamResult[int]) int64 { return 1 })
	if err != nil {
		return err
	}
	sum := 0
	for b := range batch {
		if b.Err != nil {
			return b.Err
		}
		for _, r := range b.Value {
			if r.Err != nil {
				return r.Err
			}
			sum += r.Value
		}
	}
	if sum != 20 {
		return fmt.Errorf("pipeline sum=%d", sum)
	}
	return nil
}
