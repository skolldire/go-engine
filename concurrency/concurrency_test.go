package concurrency

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
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
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not make progress")
	}
	var zero T
	return zero
}

func TestMapBoundsExecutionAndOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	input := make(chan int, 100)
	for i := range 100 {
		input <- i
	}
	close(input)
	started := make(chan struct{}, 100)
	gate := make(chan struct{})
	var active, peak atomic.Int32
	out, err := Map(ctx, input, Config{Workers: 3, Buffer: 2}, func(ctx context.Context, n int) (int, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); current > old; old = peak.Load() {
			if peak.CompareAndSwap(old, current) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-gate:
			return n * 2, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		receive(t, started)
	}
	if active.Load() != 3 || len(started) != 0 {
		t.Fatal("worker limit exceeded")
	}
	close(gate)
	// Two buffered results and three blocked workers exhaust admission. Waiting
	// for five completed calls proves the buffer bound without timing assertions.
	for range 2 {
		receive(t, started)
	}
	if cap(out) != 2 {
		t.Fatal("incorrect output capacity")
	}
	seen := map[int]bool{}
	for r := range out {
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		if seen[r.Value] {
			t.Fatal("duplicate result")
		}
		seen[r.Value] = true
	}
	if len(seen) != 100 || peak.Load() > 3 {
		t.Fatalf("results=%d peak=%d", len(seen), peak.Load())
	}
}

func TestMapCancellationUnblocksDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext(t))
	started := make(chan struct{})
	out, err := Map(ctx, From(ctx, 1), Config{Workers: 1}, func(context.Context, int) (int, error) { close(started); return 1, nil })
	if err != nil {
		t.Fatal(err)
	}
	receive(t, started)
	cancel()
	for range out {
	}
}

func TestMapTimeoutDoesNotDetachTask(t *testing.T) {
	ctx := testContext(t)
	input := make(chan int, 2)
	input <- 1
	input <- 2
	close(input)
	timedOut := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	out, err := Map(ctx, input, Config{Workers: 1, TaskTimeout: time.Millisecond}, func(ctx context.Context, n int) (int, error) {
		calls.Add(1)
		if n == 1 {
			<-ctx.Done()
			close(timedOut)
			<-release
		}
		return n, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	receive(t, timedOut)
	if calls.Load() != 1 {
		t.Fatal("timeout detached a running task")
	}
	close(release)
	first := receive(t, out)
	if !errors.Is(first.Err, context.DeadlineExceeded) {
		t.Fatalf("timeout error: %v", first.Err)
	}
	for range out {
	}
	if calls.Load() != 2 {
		t.Fatal("next task was lost")
	}
}

func TestMapContainsPanicsAndContinues(t *testing.T) {
	ctx := testContext(t)
	sentinel := errors.New("task failure")
	out, err := Map(ctx, From(ctx, 1, 2, 3), Config{Workers: 1}, func(_ context.Context, n int) (int, error) {
		if n == 1 {
			panic("boom")
		}
		if n == 2 {
			return 0, sentinel
		}
		return n, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(receive(t, out).Err, ErrPanic) {
		t.Fatal("panic not contained")
	}
	if !errors.Is(receive(t, out).Err, sentinel) {
		t.Fatal("task error lost")
	}
	if r := receive(t, out); r.Value != 3 || r.Err != nil {
		t.Fatalf("pool did not continue: %+v", r)
	}
	for range out {
	}
}

func TestFanOutFanInExactlyOnce(t *testing.T) {
	ctx := testContext(t)
	values := make([]int, 100)
	for i := range values {
		values[i] = i
	}
	streams, err := FanOut(ctx, From(ctx, values...), 4, 2)
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
			t.Fatal("duplicate item")
		}
		seen[n] = true
	}
	if len(seen) != len(values) {
		t.Fatalf("missing items: %d", len(seen))
	}
	empty, err := FanIn[int](ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := <-empty; ok {
		t.Fatal("empty merge did not close")
	}
}

func TestLimiterCancellationAndIdempotentRelease(t *testing.T) {
	ctx := testContext(t)
	l, err := NewLimiter(1)
	if err != nil {
		t.Fatal(err)
	}
	release, err := l.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	blocked, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = l.Acquire(blocked); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire: %v", err)
	}
	release()
	release()
	next, err := l.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next()
}

func TestBatchPartialAndIndependentStorage(t *testing.T) {
	ctx := testContext(t)
	out, err := Batch(ctx, From(ctx, 1, 2, 3, 4, 5), 2, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	var batches [][]int
	for b := range out {
		batches = append(batches, b)
	}
	if len(batches) != 3 || len(batches[2]) != 1 || batches[2][0] != 5 {
		t.Fatalf("batches: %v", batches)
	}
	batches[1][0] = 99
	if batches[0][0] != 1 {
		t.Fatal("batch storage was reused")
	}
}

func TestBatchFlushesOnTimerAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	input := make(chan int, 1)
	input <- 42
	out, err := Batch(ctx, input, 10, time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	if b := receive(t, out); len(b) != 1 || b[0] != 42 {
		t.Fatalf("timed batch: %v", b)
	}
	cancel()
	for range out {
	}
}

func TestCancelledStreamsClose(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext(t))
	source := From(ctx, 1, 2, 3)
	streams, err := FanOut(ctx, source, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := FanIn(ctx, 0, streams...)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	for range merged {
	}
	for _, s := range streams {
		for range s {
		}
	}
	for range source {
	}
}

func TestInvalidConfiguration(t *testing.T) {
	ctx := testContext(t)
	in := make(chan int)
	fn := func(context.Context, int) (int, error) { return 0, nil }
	for _, cfg := range []Config{{}, {Workers: -1}, {Workers: 1, Buffer: -1}, {Workers: 1, TaskTimeout: -1}} {
		if _, err := Map(ctx, in, cfg, fn); !errors.Is(err, ErrInvalidConfig) {
			t.Fatal("invalid map config accepted")
		}
	}
	if _, err := Map(ctx, in, Config{Workers: 1}, (func(context.Context, int) (int, error))(nil)); err == nil {
		t.Fatal("nil function accepted")
	}
	if _, err := Batch(ctx, in, 0, 0, 0); err == nil {
		t.Fatal("invalid batch accepted")
	}
	if _, err := FanOut(ctx, in, 0, 0); err == nil {
		t.Fatal("invalid fan-out accepted")
	}
	if _, err := FanIn[int](ctx, -1); err == nil {
		t.Fatal("invalid fan-in accepted")
	}
	if _, err := NewLimiter(0); err == nil {
		t.Fatal("invalid limiter accepted")
	}
}

func TestLimiterCancelsWaitingAcquire(t *testing.T) {
	ctx := testContext(t)
	limiter, err := NewLimiter(1)
	if err != nil {
		t.Fatal(err)
	}
	release, err := limiter.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	waiting, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		r, err := limiter.Acquire(waiting)
		if err == nil {
			r()
		}
		done <- err
	}()
	cancel()
	if err := receive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting acquire: %v", err)
	}
}

func TestMapCancelledBeforeStartDoesNotExecute(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	input := make(chan int, 1)
	input <- 1
	close(input)
	var calls atomic.Int32
	out, err := Map(ctx, input, Config{Workers: 4}, func(context.Context, int) (int, error) { calls.Add(1); return 0, nil })
	if err != nil {
		t.Fatal(err)
	}
	for range out {
	}
	if calls.Load() != 0 {
		t.Fatal("started a task after cancellation")
	}
}
