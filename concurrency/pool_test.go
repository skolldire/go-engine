package concurrency

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func poolConfig(workers, queue int) PoolConfig {
	c := DefaultPoolConfig()
	c.Workers = workers
	c.QueueCapacity = queue
	c.MaxJobWeight = 16
	c.MaxInFlightWeight = 128
	return c
}
func shutdownPool[I, O any](t *testing.T, p *Pool[I, O]) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := p.Shutdown(ctx, ShutdownDrain); err != nil {
		t.Fatal(err)
	}
}
func outcome[O any](t *testing.T, f *Future[O]) Result[O] {
	t.Helper()
	r, err := f.Wait(testContext(t))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func submitInt(t *testing.T, p *Pool[int, int], ctx context.Context, id, key string, n int) *Future[int] {
	t.Helper()
	f, err := p.Submit(ctx, Job[int]{ID: id, Key: key, Weight: 1, Value: n})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func assertInvariant(t *testing.T, s Stats, c PoolConfig) {
	t.Helper()
	if s.Running > c.Workers || s.Queued > c.QueueCapacity || s.InFlightWeight > c.MaxInFlightWeight || s.InFlightWeight < 0 || s.Queued < 0 || s.Running < 0 {
		t.Fatalf("bounds: %+v", s)
	}
	if s.Accepted != s.Succeeded+s.Failed+s.Canceled+uint64(s.Queued+s.Running) {
		t.Fatalf("accounting: %+v", s)
	}
	if c.Ordering == OrderingPerKey && s.ActiveKeys > c.MaxActiveKeys {
		t.Fatalf("keys: %+v", s)
	}
}

func TestPoolAdmissionAndIdentity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := poolConfig(1, 1)
		cfg.Admission = AdmissionReject
		start := make(chan struct{})
		gate := make(chan struct{})
		p, err := NewPool(cfg, func(_ context.Context, n int) (int, error) {
			if n == 1 {
				close(start)
				<-gate
			}
			return n * 2, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		a := submitInt(t, p, context.Background(), "a", "", 1)
		<-start
		b := submitInt(t, p, context.Background(), "b", "", 2)
		if f, err := p.Submit(context.Background(), Job[int]{ID: "a", Weight: 1}); f != nil || !errors.Is(err, ErrDuplicateID) {
			t.Fatalf("duplicate: %v", err)
		}
		if f, err := p.Submit(context.Background(), Job[int]{ID: "c", Weight: 1}); f != nil || !errors.Is(err, ErrSaturated) {
			t.Fatalf("saturated: %v", err)
		}
		assertInvariant(t, p.Stats(), cfg)
		close(gate)
		if r := outcome(t, a); r.Value != 2 || r.State != StateSucceeded || r.Sequence != 1 {
			t.Fatalf("first: %+v", r)
		}
		if r := outcome(t, b); r.Value != 4 || r.Sequence != 2 {
			t.Fatalf("second: %+v", r)
		}
		again := submitInt(t, p, context.Background(), "a", "", 3)
		if r := outcome(t, again); r.Sequence != 3 {
			t.Fatalf("ID reuse: %+v", r)
		}
		shutdownPool(t, p)
		assertInvariant(t, p.Stats(), cfg)
		if _, err := p.Submit(context.Background(), Job[int]{ID: "closed", Weight: 1}); !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
	})
}

func TestPoolAdmissionTimeoutAndWeight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := poolConfig(2, 2)
		c.MaxJobWeight = math.MaxInt64
		c.MaxInFlightWeight = math.MaxInt64
		c.AdmissionTimeout = time.Second
		gate := make(chan struct{})
		p, err := NewPool(c, func(context.Context, int) (int, error) { <-gate; return 1, nil })
		if err != nil {
			t.Fatal(err)
		}
		f, err := p.Submit(context.Background(), Job[int]{ID: "heavy", Weight: math.MaxInt64})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p.Submit(context.Background(), Job[int]{ID: "overflow", Weight: 1}); !errors.Is(err, ErrAdmissionTimeout) {
			t.Fatalf("weight overflow: %v", err)
		}
		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err = p.Submit(canceled, Job[int]{ID: "canceled", Weight: 1}); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		close(gate)
		outcome(t, f)
		shutdownPool(t, p)
	})
}

func TestPoolQueuedCancellationReleasesResources(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := poolConfig(1, 1)
		gate := make(chan struct{})
		start := make(chan struct{})
		var calls atomic.Int32
		p, err := NewPool(c, func(_ context.Context, n int) (int, error) {
			calls.Add(1)
			if n == 1 {
				close(start)
				<-gate
			}
			return n, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		first := submitInt(t, p, context.Background(), "first", "", 1)
		<-start
		ctx, cancel := context.WithCancel(context.Background())
		queued := submitInt(t, p, ctx, "queued", "", 2)
		cancel()
		r := outcome(t, queued)
		if r.State != StateCanceled || !r.StartedAt.IsZero() || !errors.Is(r.Err, context.Canceled) {
			t.Fatalf("queued result: %+v", r)
		}
		if p.Stats().Queued != 0 || p.Stats().InFlightWeight != 1 {
			t.Fatalf("resources: %+v", p.Stats())
		}
		third := submitInt(t, p, context.Background(), "third", "", 3)
		close(gate)
		outcome(t, first)
		outcome(t, third)
		shutdownPool(t, p)
		if calls.Load() != 2 {
			t.Fatal("canceled queued job executed")
		}
	})
}

func TestFutureWaitCancelAndTerminalPrecedence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := make(chan struct{})
		gate := make(chan struct{})
		domain := errors.New("domain")
		cause := errors.New("operator")
		p, err := NewPool(DefaultPoolConfig(), func(ctx context.Context, _ int) (int, error) { close(start); <-gate; return 42, domain })
		if err != nil {
			t.Fatal(err)
		}
		f := submitInt(t, p, context.Background(), "active", "", 1)
		<-start
		waitCtx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err = f.Wait(waitCtx); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if p.Stats().Running != 1 {
			t.Fatal("Wait canceled task")
		}
		f.Cancel(cause)
		select {
		case <-f.Done():
			t.Fatal("active task was detached")
		default:
		}
		close(gate)
		r := outcome(t, f)
		if r.State != StateCanceled || !errors.Is(r.Err, cause) || !errors.Is(r.Err, domain) {
			t.Fatalf("causes: %+v", r)
		}
		f.Cancel(errors.New("too late"))
		r2 := outcome(t, f)
		if r2.Err != r.Err || r2.FinishedAt != r.FinishedAt {
			t.Fatal("terminal outcome changed")
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				rr, e := f.Wait(context.Background())
				if e != nil || rr.Sequence != r.Sequence {
					t.Error("concurrent wait")
				}
			})
		}
		wg.Wait()
		shutdownPool(t, p)
	})
}

func TestPoolTaskTimeoutAndShutdownEscalation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := poolConfig(1, 2)
		cfg.TaskTimeout = time.Second
		timedOut := make(chan struct{})
		release := make(chan struct{})
		var calls atomic.Int32
		p, err := NewPool(cfg, func(ctx context.Context, n int) (int, error) {
			calls.Add(1)
			<-ctx.Done()
			close(timedOut)
			<-release
			return n, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		f := submitInt(t, p, context.Background(), "active", "", 1)
		<-timedOut
		queued := submitInt(t, p, context.Background(), "queued", "", 2)
		short, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err = p.Shutdown(short, ShutdownDrain); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("drain timeout: %v", err)
		}
		short2, cancel2 := context.WithTimeout(context.Background(), time.Second)
		defer cancel2()
		if err = p.Shutdown(short2, ShutdownCancel); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cancel timeout: %v", err)
		}
		if outcome(t, queued).State != StateCanceled {
			t.Fatal("queued not canceled")
		}
		select {
		case <-p.Done():
			t.Fatal("pool claimed a noncooperative worker stopped")
		default:
		}
		if p.Stats().Running != 1 || p.Stats().InFlightWeight != 1 {
			t.Fatal("slot released early")
		}
		close(release)
		r := outcome(t, f)
		if r.State != StateCanceled || !errors.Is(r.Err, context.DeadlineExceeded) {
			t.Fatalf("timeout: %+v", r)
		}
		shutdownPool(t, p)
		if calls.Load() != 1 {
			t.Fatal("replacement executed")
		}
	})
}

func TestPoolKeyOrderRoundRobinAndFailure(t *testing.T) {
	for _, policy := range []KeyFailurePolicy{KeyFailureContinue, KeyFailureCancelPending} {
		t.Run(string(policy), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := poolConfig(1, 8)
				c.Ordering = OrderingPerKey
				c.MaxActiveKeys = 3
				c.PerKeyCapacity = 4
				c.KeyFailure = policy
				started := make(chan int, 8)
				gate := make(chan struct{})
				p, err := NewPool(c, func(_ context.Context, n int) (int, error) {
					started <- n
					if n == 1 {
						<-gate
						return 0, errors.New("first fails")
					}
					return n, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				a := submitInt(t, p, context.Background(), "a1", "a", 1)
				<-started
				a2 := submitInt(t, p, context.Background(), "a2", "a", 2)
				a3 := submitInt(t, p, context.Background(), "a3", "a", 3)
				b := submitInt(t, p, context.Background(), "b1", "b", 4)
				cc := submitInt(t, p, context.Background(), "c1", "c", 5)
				close(gate)
				outcome(t, a)
				outcome(t, b)
				outcome(t, cc)
				if x, y := <-started, <-started; x != 4 || y != 5 {
					t.Fatalf("round robin: %d %d", x, y)
				}
				if policy == KeyFailureContinue {
					if (<-started) != 2 || (<-started) != 3 {
						t.Fatal("key order")
					}
					outcome(t, a2)
					outcome(t, a3)
				} else {
					for _, f := range []*Future[int]{a2, a3} {
						r := outcome(t, f)
						if !errors.Is(r.Err, ErrKeyAborted) || !r.StartedAt.IsZero() {
							t.Fatalf("key abort: %+v", r)
						}
					}
				}
				fresh := submitInt(t, p, context.Background(), "a4", "a", 6)
				outcome(t, fresh)
				shutdownPool(t, p)
				if p.Stats().ActiveKeys != 0 {
					t.Fatal("keys retained")
				}
				assertInvariant(t, p.Stats(), c)
			})
		})
	}
}

func TestPoolKeyBoundsAndParallelKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := poolConfig(3, 4)
		c.Ordering = OrderingPerKey
		c.MaxActiveKeys = 2
		c.PerKeyCapacity = 1
		c.KeyFailure = KeyFailureContinue
		c.Admission = AdmissionReject
		gates := make(chan struct{})
		started := make(chan int, 3)
		p, err := NewPool(c, func(_ context.Context, n int) (int, error) { started <- n; <-gates; return n, nil })
		if err != nil {
			t.Fatal(err)
		}
		a := submitInt(t, p, context.Background(), "a", "a", 1)
		<-started
		b := submitInt(t, p, context.Background(), "b", "b", 2)
		<-started
		aq := submitInt(t, p, context.Background(), "aq", "a", 3)
		for _, job := range []Job[int]{{ID: "keys", Key: "c", Weight: 1}, {ID: "per-key", Key: "a", Weight: 1}} {
			if _, err = p.Submit(context.Background(), job); !errors.Is(err, ErrSaturated) {
				t.Fatalf("key bound: %v", err)
			}
		}
		if p.Stats().Running != 2 || p.Stats().Queued != 1 {
			t.Fatal(p.Stats())
		}
		close(gates)
		outcome(t, a)
		outcome(t, b)
		outcome(t, aq)
		shutdownPool(t, p)
	})
}

func TestPoolPanicAndValidation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		if _, err := NewPool[int, int](DefaultPoolConfig(), nil); !errors.Is(err, ErrInvalidConfig) {
			t.Fatal(err)
		}
		p, err := NewPool(DefaultPoolConfig(), func(context.Context, int) (int, error) { panic("contained") })
		if err != nil {
			t.Fatal(err)
		}
		for _, job := range []Job[int]{{Weight: 1}, {ID: "bad", Weight: 0}, {ID: "bad", Key: "not-allowed", Weight: 1}} {
			if _, err = p.Submit(context.Background(), job); !errors.Is(err, ErrInvalidConfig) {
				t.Fatal(err)
			}
		}
		//nolint:staticcheck // SA1012: deliberately verify the nil-context contract.
		if _, err = p.Submit(nil, Job[int]{ID: "nil", Weight: 1}); !errors.Is(err, ErrInvalidConfig) {
			t.Fatal(err)
		}
		if _, err = p.Submit(context.Background(), Job[int]{ID: "large", Weight: 2}); !errors.Is(err, ErrJobTooLarge) {
			t.Fatal(err)
		}
		f := submitInt(t, p, context.Background(), "panic", "", 1)
		r := outcome(t, f)
		var pe *PanicError
		if r.State != StateFailed || !errors.As(r.Err, &pe) || !errors.Is(r.Err, ErrPanic) || len(pe.Stack) == 0 || len(pe.Stack) > 64<<10 {
			t.Fatalf("panic: %+v", r)
		}
		shutdownPool(t, p)
	})
}

func TestPoolConcurrentAdmissionAndShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := poolConfig(4, 8)
		p, err := NewPool(c, func(ctx context.Context, n int) (int, error) {
			<-ctx.Done()
			return n, ctx.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := range 40 {
			wg.Go(func() {
				f, e := p.Submit(context.Background(), Job[int]{ID: fmt.Sprint(i), Weight: 1, Value: i})
				if e == nil {
					outcome(t, f)
				} else if !errors.Is(e, ErrClosed) {
					t.Error(e)
				}
				assertInvariant(t, p.Stats(), c)
			})
		}
		synctest.Wait()
		for i := range 4 {
			wg.Go(func() {
				mode := ShutdownDrain
				if i%2 == 1 {
					mode = ShutdownCancel
				}
				if e := p.Shutdown(context.Background(), mode); e != nil {
					t.Error(e)
				}
			})
		}
		wg.Wait()
		assertInvariant(t, p.Stats(), c)
	})
}

func TestAdmissionTimerDoesNotCancelAcceptedJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := DefaultPoolConfig()
		cfg.AdmissionTimeout = time.Second
		gate := make(chan struct{})
		started := make(chan struct{})
		p, err := NewPool(cfg, func(ctx context.Context, n int) (int, error) { close(started); <-gate; return n, ctx.Err() })
		if err != nil {
			t.Fatal(err)
		}
		f := submitInt(t, p, context.Background(), "accepted", "", 7)
		<-started
		synctest.Sleep(2 * time.Second)
		close(gate)
		if r := outcome(t, f); r.State != StateSucceeded || r.Err != nil {
			t.Fatalf("admission timer leaked into task %+v", r)
		}
		shutdownPool(t, p)
	})
}
func TestPoolBlockedSubmitWakesOnShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		started := make(chan struct{})
		p, err := NewPool(DefaultPoolConfig(), func(context.Context, int) (int, error) { close(started); <-gate; return 1, nil })
		if err != nil {
			t.Fatal(err)
		}
		f := submitInt(t, p, context.Background(), "first", "", 1)
		<-started
		blocked := make(chan error, 1)
		go func() { _, e := p.Submit(context.Background(), Job[int]{ID: "blocked", Weight: 1}); blocked <- e }()
		synctest.Wait()
		wait, cancel := context.WithCancel(context.Background())
		cancel()
		if err := p.Shutdown(wait, ShutdownDrain); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if err := <-blocked; !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
		close(gate)
		outcome(t, f)
		shutdownPool(t, p)
	})
}
func TestKeyCancellationDoesNotApplyFailurePolicy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := poolConfig(1, 1)
		c.Ordering = OrderingPerKey
		c.MaxActiveKeys = 1
		c.PerKeyCapacity = 1
		c.KeyFailure = KeyFailureCancelPending
		start := make(chan struct{})
		p, err := NewPool(c, func(ctx context.Context, n int) (int, error) {
			if n == 1 {
				close(start)
				<-ctx.Done()
				return 0, ctx.Err()
			}
			return n, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		first := submitInt(t, p, context.Background(), "a", "key", 1)
		<-start
		second := submitInt(t, p, context.Background(), "b", "key", 2)
		first.Cancel(nil)
		if r := outcome(t, first); r.State != StateCanceled {
			t.Fatal(r)
		}
		if r := outcome(t, second); r.State != StateSucceeded {
			t.Fatal(r)
		}
		shutdownPool(t, p)
	})
}
func TestPanicWinsConcurrentCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		p, err := NewPool(DefaultPoolConfig(), func(context.Context, int) (int, error) { close(started); <-release; panic("boom") })
		if err != nil {
			t.Fatal(err)
		}
		f := submitInt(t, p, context.Background(), "panic", "", 1)
		<-started
		f.Cancel(nil)
		close(release)
		if r := outcome(t, f); r.State != StateFailed || !errors.Is(r.Err, ErrPanic) || !errors.Is(r.Err, context.Canceled) {
			t.Fatalf("precedence %+v", r)
		}
		shutdownPool(t, p)
	})
}

func TestClosedShutdownAndInvalidHandles(t *testing.T) {
	p, err := NewPool(DefaultPoolConfig(), func(context.Context, int) (int, error) { return 0, nil })
	if err != nil {
		t.Fatal(err)
	}
	//nolint:staticcheck // SA1012: deliberately verify nil-context rejection.
	if err := p.Shutdown(nil, ShutdownDrain); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	if err := p.Shutdown(context.Background(), ShutdownMode("invalid")); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	shutdownPool(t, p)
	if err := p.Shutdown(context.Background(), ShutdownMode("irrelevant-after-close")); err != nil {
		t.Fatal(err)
	}
	var zeroPool Pool[int, int]
	if _, err := zeroPool.Submit(context.Background(), Job[int]{ID: "x", Weight: 1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	if err := zeroPool.Shutdown(context.Background(), ShutdownDrain); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	var zeroFuture Future[int]
	if _, err := zeroFuture.Wait(context.Background()); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	zeroFuture.Cancel(nil)
}

type externallyCanceledContext struct {
	context.Context
	signal <-chan struct{}
}

func (c externallyCanceledContext) Done() <-chan struct{} { return c.signal }
func (c externallyCanceledContext) Err() error {
	select {
	case <-c.signal:
		return context.Canceled
	default:
		return nil
	}
}

type testContextKey struct{}

func TestPoolCustomParentAndNestedDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		signal := make(chan struct{})
		parent := externallyCanceledContext{Context: context.WithValue(context.Background(), testContextKey{}, "preserved"), signal: signal}
		started := make(chan struct{})
		p, err := NewPool(DefaultPoolConfig(), func(ctx context.Context, n int) (int, error) {
			if ctx.Value(testContextKey{}) != "preserved" {
				t.Error("context values lost")
			}
			close(started)
			<-ctx.Done()
			return n, ctx.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
		f := submitInt(t, p, parent, "custom", "", 1)
		<-started
		close(signal)
		if r := outcome(t, f); r.State != StateCanceled || !errors.Is(r.Err, context.Canceled) {
			t.Fatalf("custom cancel %+v", r)
		}
		shutdownPool(t, p)
		deadline, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		p, err = NewPool(DefaultPoolConfig(), func(ctx context.Context, n int) (int, error) {
			derived, stop := context.WithCancel(ctx)
			defer stop()
			<-derived.Done()
			if derived.Err() != context.DeadlineExceeded {
				t.Errorf("nested deadline Err: %v", derived.Err())
			}
			return n, derived.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
		f = submitInt(t, p, deadline, "deadline", "", 1)
		if r := outcome(t, f); r.State != StateCanceled || !errors.Is(r.Err, context.DeadlineExceeded) {
			t.Fatalf("parent deadline %+v", r)
		}
		shutdownPool(t, p)
	})
}

func TestPoolPreservesParentDeadlineCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reason := errors.New("request budget exhausted")
		parent, cancel := context.WithDeadlineCause(context.Background(), time.Now().Add(time.Second), reason)
		defer cancel()
		p, err := NewPool(DefaultPoolConfig(), func(ctx context.Context, n int) (int, error) {
			<-ctx.Done()
			if !errors.Is(context.Cause(ctx), reason) {
				t.Errorf("task lost parent cause: %v", context.Cause(ctx))
			}
			return n, ctx.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
		defer shutdownPool(t, p)
		f := submitInt(t, p, parent, "deadline-cause", "", 1)
		r := outcome(t, f)
		if r.State != StateCanceled || !errors.Is(r.Err, reason) || !errors.Is(r.Err, context.DeadlineExceeded) {
			t.Fatalf("parent cause not preserved: %+v", r)
		}
		shutdownPool(t, p)
	})
}

type delayedErrContext struct {
	context.Context
	once sync.Once
}

func (c *delayedErrContext) Err() error {
	c.once.Do(func() { synctest.Sleep(2 * time.Second) })
	return c.Context.Err()
}
func TestRejectAdmissionIgnoresBlockingTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := DefaultPoolConfig()
		cfg.Admission = AdmissionReject
		cfg.AdmissionTimeout = time.Second
		gate := make(chan struct{})
		p, err := NewPool(cfg, func(context.Context, int) (int, error) { <-gate; return 1, nil })
		if err != nil {
			t.Fatal(err)
		}
		defer func() { close(gate); shutdownPool(t, p) }()
		synctest.Wait()
		ctx := &delayedErrContext{Context: context.Background()}
		first, err := p.Submit(ctx, Job[int]{ID: "available", Weight: 1})
		if err != nil {
			t.Fatalf("reject policy applied blocking timeout: %v", err)
		}
		if _, err := p.Submit(context.Background(), Job[int]{ID: "saturated", Weight: 1}); !errors.Is(err, ErrSaturated) {
			t.Fatalf("reject saturation: %v", err)
		}
		if first == nil {
			t.Fatal("accepted job lacks future")
		}
	})
}
