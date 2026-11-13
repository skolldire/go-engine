package main

import (
	"context"
	"errors"
	"fmt"
	c "github.com/skolldire/go-engine/concurrency"
	"time"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("minimal: Pool/Future, streams, Batch, Limiter, cancellation OK")
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := c.DefaultPoolConfig()
	pool, err := c.NewPool(cfg, func(_ context.Context, n int) (int, error) { return n * 2, nil })
	if err != nil {
		return err
	}
	defer func() { _ = pool.Shutdown(ctx, c.ShutdownCancel) }()
	future, err := pool.Submit(ctx, c.Job[int]{ID: "answer", Weight: 1, Value: 21})
	if err != nil {
		return err
	}
	r, err := future.Wait(ctx)
	if err != nil {
		return err
	}
	if r.Err != nil || r.ID != "answer" || r.Value != 42 || r.State != c.StateSucceeded {
		return fmt.Errorf("invalid outcome: %+v", r)
	}
	if err := pool.Shutdown(ctx, c.ShutdownDrain); err != nil {
		return err
	}
	if _, err := pool.Submit(ctx, c.Job[int]{ID: "late", Weight: 1}); !errors.Is(err, c.ErrClosed) {
		return fmt.Errorf("closed admission: %w", err)
	}
	input, err := c.From(ctx, 1, 2, 3)
	if err != nil {
		return err
	}
	mapped, err := c.Map(ctx, input, c.StreamConfig{Workers: 1, Buffer: 1}, func(_ context.Context, n int) (int, error) { return n * n, nil })
	if err != nil {
		return err
	}
	batches, err := c.Batch(ctx, mapped, c.BatchConfig{Size: 2, MaxBatchWeight: 2}, func(c.StreamResult[int]) int64 { return 1 })
	if err != nil {
		return err
	}
	sum := 0
	for batch := range batches {
		if batch.Err != nil {
			return batch.Err
		}
		for _, item := range batch.Value {
			if item.Err != nil {
				return item.Err
			}
			sum += item.Value
		}
	}
	if sum != 14 {
		return fmt.Errorf("stream sum=%d", sum)
	}
	in, err := c.From(ctx, 1, 2, 3, 4)
	if err != nil {
		return err
	}
	split, err := c.FanOut(ctx, in, 2, 1)
	if err != nil {
		return err
	}
	joined, err := c.FanIn(ctx, 1, split...)
	if err != nil {
		return err
	}
	sum = 0
	for n := range joined {
		sum += n
	}
	if sum != 10 {
		return fmt.Errorf("fan-in sum=%d", sum)
	}
	limiter, err := c.NewLimiter(c.LimiterConfig{Capacity: 2})
	if err != nil {
		return err
	}
	release, err := limiter.Acquire(ctx, 2)
	if err != nil {
		return err
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := limiter.Acquire(canceled, 1); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("cancel wait: %w", err)
	}
	release()
	release()
	if _, err := c.From[int](nil); !errors.Is(err, c.ErrInvalidConfig) {
		return fmt.Errorf("nil ctx: %w", err)
	}
	return nil
}
