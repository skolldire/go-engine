package concurrency

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not progress")
	}
	var z T
	return z
}
func source[T any](t *testing.T, ctx context.Context, values ...T) <-chan T {
	t.Helper()
	in, err := From(ctx, values...)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestMapBoundsAndPanic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := testContext(t)
		gate := make(chan struct{})
		values := []int{1, 2, 3, 4, 5, 6}
		start := make(chan struct{}, len(values))
		var active, peak atomic.Int32
		in := source(t, ctx, values...)
		out, err := Map(ctx, in, StreamConfig{Workers: 3, Buffer: 2}, func(_ context.Context, n int) (int, error) {
			current := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); current > old; old = peak.Load() {
				if peak.CompareAndSwap(old, current) {
					break
				}
			}
			start <- struct{}{}
			<-gate
			if n == 2 {
				panic("contained")
			}
			return n * 2, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for range 3 {
			<-start
		}
		synctest.Wait()
		if active.Load() != 3 {
			t.Fatal("worker bound")
		}
		close(gate)
		count := 0
		panics := 0
		for r := range out {
			count++
			if errors.Is(r.Err, ErrPanic) {
				panics++
			}
		}
		if count != 6 || panics != 1 || peak.Load() > 3 || cap(out) != 2 {
			t.Fatalf("count=%d panic=%d peak=%d", count, panics, peak.Load())
		}
	})
}
func TestMapDeadlineAndAbandonedOutput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(testContext(t))
		defer cancel()
		out, err := Map(ctx, source(t, ctx, 1), StreamConfig{Workers: 1, TaskTimeout: time.Second}, func(ctx context.Context, n int) (int, error) { <-ctx.Done(); return n, nil })
		if err != nil {
			t.Fatal(err)
		}
		if r := receive(t, out); !errors.Is(r.Err, context.DeadlineExceeded) {
			t.Fatalf("deadline %v", r.Err)
		}
		for range out {
		}
		started := make(chan struct{})
		out, err = Map(ctx, source(t, ctx, 1), StreamConfig{Workers: 1}, func(context.Context, int) (int, error) { close(started); return 1, nil })
		if err != nil {
			t.Fatal(err)
		}
		<-started
		synctest.Wait()
		cancel()
		for range out {
		}
	})
}
func TestFanOutFanInConservation(t *testing.T) {
	ctx := testContext(t)
	values := make([]int, 100)
	for i := range values {
		values[i] = i
	}
	streams, err := FanOut(ctx, source(t, ctx, values...), 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	out, err := FanIn(ctx, 3, append(streams, nil)...)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for n := range out {
		if seen[n] {
			t.Fatal("duplicate")
		}
		seen[n] = true
	}
	if len(seen) != 100 {
		t.Fatal(len(seen))
	}
	empty, err := FanIn[int](ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := <-empty; ok {
		t.Fatal("empty merge")
	}
}
func TestBatchSizeWeightAndStorage(t *testing.T) {
	ctx := testContext(t)
	out, err := Batch(ctx, source(t, ctx, 2, 3, 1, 5, 1), BatchConfig{Size: 3, Buffer: 1, MaxBatchWeight: 5}, func(n int) int64 { return int64(n) })
	if err != nil {
		t.Fatal(err)
	}
	var batches [][]int
	for r := range out {
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		batches = append(batches, r.Value)
	}
	if len(batches) != 4 || len(batches[0]) != 2 || batches[3][0] != 1 {
		t.Fatalf("batches %v", batches)
	}
	batches[1][0] = 99
	if batches[0][0] != 2 {
		t.Fatal("shared backing storage")
	}
}
func TestBatchTimerErrorsAndCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(testContext(t))
		defer cancel()
		in := make(chan int, 1)
		in <- 42
		out, err := Batch(ctx, in, BatchConfig{Size: 10, MaxWait: time.Second, MaxBatchWeight: 100}, func(n int) int64 { return int64(n) })
		if err != nil {
			t.Fatal(err)
		}
		if r := receive(t, out); len(r.Value) != 1 || r.Value[0] != 42 {
			t.Fatalf("timer %+v", r)
		}
		cancel()
		for range out {
		}
	})
	for _, value := range []int{0, 6} {
		ctx, cancel := context.WithCancel(testContext(t))
		out, err := Batch(ctx, source(t, ctx, value), BatchConfig{Size: 2, MaxBatchWeight: 5}, func(n int) int64 { return int64(n) })
		if err != nil {
			t.Fatal(err)
		}
		r := receive(t, out)
		want := ErrInvalidConfig
		if value == 6 {
			want = ErrJobTooLarge
		}
		if !errors.Is(r.Err, want) {
			t.Fatal(r.Err)
		}
		if _, ok := <-out; ok {
			t.Fatal("terminal error did not close")
		}
		cancel()
	}
	ctx := testContext(t)
	out, err := Batch(ctx, source(t, ctx, int64(math.MaxInt64), int64(1)), BatchConfig{Size: 2, MaxBatchWeight: math.MaxInt64}, func(n int64) int64 { return n })
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for r := range out {
		count += len(r.Value)
	}
	if count != 2 {
		t.Fatal(count)
	}
}
func TestLimiterWeightedFIFOAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, err := NewLimiter(LimiterConfig{Capacity: 3})
		if err != nil {
			t.Fatal(err)
		}
		initial, err := l.Acquire(context.Background(), 2)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		bigDone := make(chan error, 1)
		go func() {
			r, e := l.Acquire(ctx, 3)
			if e == nil {
				r()
			}
			bigDone <- e
		}()
		synctest.Wait()
		smallDone := make(chan func(), 1)
		go func() {
			r, e := l.Acquire(context.Background(), 1)
			if e != nil {
				t.Error(e)
				return
			}
			smallDone <- r
		}()
		synctest.Wait()
		select {
		case <-smallDone:
			t.Fatal("small overtook head")
		default:
		}
		cancel()
		if e := <-bigDone; !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
		small := <-smallDone
		small()
		small()
		initial()
		initial()
		full, err := l.Acquire(context.Background(), 3)
		if err != nil {
			t.Fatal(err)
		}
		full()
	})
}
func TestCanceledStreamsClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		in := source(t, ctx, 1, 2, 3)
		outputs, err := FanOut(ctx, in, 2, 0)
		if err != nil {
			t.Fatal(err)
		}
		out, err := FanIn(ctx, 0, outputs...)
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		cancel()
		for range out {
		}
		for _, s := range outputs {
			for range s {
			}
		}
		for range in {
		}
	})
}

//nolint:staticcheck // SA1012: nil-context rejection is explicitly part of the public contract.
func TestInvalidStreamingAndLimiter(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)
	fn := func(context.Context, int) (int, error) { return 0, nil }
	if _, err := From[int](nil); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	for _, cfg := range []StreamConfig{{}, {Workers: 1, Buffer: -1}, {Workers: 1, TaskTimeout: -1}} {
		if _, err := Map(ctx, in, cfg, fn); !errors.Is(err, ErrInvalidConfig) {
			t.Fatal(err)
		}
	}
	if _, err := Map(ctx, in, StreamConfig{Workers: 1}, (Task[int, int])(nil)); err == nil {
		t.Fatal("nil task")
	}
	if _, err := Map[int, int](nil, in, StreamConfig{Workers: 1}, fn); err == nil {
		t.Fatal("nil ctx")
	}
	if _, err := Map[int, int](ctx, nil, StreamConfig{Workers: 1}, fn); err == nil {
		t.Fatal("nil input")
	}
	if _, err := FanOut(ctx, in, 0, 0); err == nil {
		t.Fatal("fanout count")
	}
	if _, err := FanIn[int](nil, 0); err == nil {
		t.Fatal("fanin context")
	}
	if _, err := FanIn[int](ctx, -1); err == nil {
		t.Fatal("fanin buffer")
	}
	if _, err := Batch(ctx, in, BatchConfig{}, func(int) int64 { return 1 }); err == nil {
		t.Fatal("batch config")
	}
	if _, err := NewLimiter(LimiterConfig{}); err == nil {
		t.Fatal("limiter capacity")
	}
	var l Limiter
	if _, err := l.Acquire(ctx, 1); err == nil {
		t.Fatal("zero limiter")
	}
	var nilLimiter *Limiter
	if _, err := nilLimiter.Acquire(ctx, 1); err == nil {
		t.Fatal("nil limiter")
	}
	limiter, _ := NewLimiter(LimiterConfig{Capacity: 1})
	for _, w := range []int64{0, -1, 2} {
		if _, err := limiter.Acquire(ctx, w); err == nil {
			t.Fatal("weight accepted")
		}
	}
	if _, err := limiter.Acquire(nil, 1); err == nil {
		t.Fatal("nil context")
	}
}
func TestLimiterConcurrentRelease(t *testing.T) {
	l, _ := NewLimiter(LimiterConfig{Capacity: 4})
	var wg sync.WaitGroup
	var active atomic.Int32
	for range 40 {
		wg.Go(func() {
			r, e := l.Acquire(testContext(t), 1)
			if e != nil {
				t.Error(e)
				return
			}
			if active.Add(1) > 4 {
				t.Error("capacity exceeded")
			}
			active.Add(-1)
			r()
			r()
		})
	}
	wg.Wait()
}
