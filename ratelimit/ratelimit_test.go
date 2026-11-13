package ratelimit

import (
	"context"
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func newTestLimiter(t *testing.T, cfg Config) *Limiter {
	t.Helper()
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func startWait(l *Limiter, ctx context.Context, n int) <-chan error {
	done := make(chan error, 1)
	go func() { done <- l.Wait(ctx, n) }()
	return done
}
func timerOf(l *Limiter) *time.Timer { l.mu.Lock(); defer l.mu.Unlock(); return l.timer }
func pending(t *testing.T, l *Limiter, want int) []*waiter {
	t.Helper()
	synctest.Wait()
	l.mu.Lock()
	defer l.mu.Unlock()
	var list []*waiter
	var previous *waiter
	for w := l.head; w != nil; w = w.next {
		if w.prev != previous {
			t.Fatal("broken queue")
		}
		previous = w
		list = append(list, w)
	}
	if len(list) != want || previous != l.tail {
		t.Fatal("pending", len(list), want)
	}
	return list
}
func TestConfigAndValidation(t *testing.T) {
	cases := []struct {
		cfg   Config
		field string
	}{{Config{}, "RatePerSecond"}, {Config{-1, 0}, "RatePerSecond"}, {Config{math.NaN(), 1}, "RatePerSecond"}, {Config{math.Inf(1), 1}, "RatePerSecond"}, {Config{math.Inf(-1), 1}, "RatePerSecond"}, {Config{1, 0}, "Burst"}, {Config{1, -1}, "Burst"}}
	for _, tc := range cases {
		l, err := New(tc.cfg)
		var ce *ConfigError
		if l != nil || !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tc.field {
			t.Fatal(l, err)
		}
	}
	for _, rate := range []float64{math.SmallestNonzeroFloat64, 0.1, 1, math.MaxFloat64} {
		l := newTestLimiter(t, Config{rate, math.MaxInt})
		if l.tokens != math.MaxInt || l.head != nil || l.timer != nil {
			t.Fatal("construction effects")
		}
	}
	cfg := Config{3, 7}
	l := newTestLimiter(t, cfg)
	cfg.Burst = 99
	cfg.RatePerSecond = 10
	if l.cfg != (Config{3, 7}) {
		t.Fatal("configuration not copied")
	}
	var nilLimiter *Limiter
	zero := new(Limiter)
	for _, limiter := range []*Limiter{nilLimiter, zero, l} {
		for _, n := range []int{-1, 0, 8} {
			if limiter.Allow(n) {
				t.Fatal("invalid Allow")
			}
		}
	}
	casesCall := []struct {
		ctx   context.Context
		l     *Limiter
		n     int
		field string
	}{{nil, nilLimiter, 0, "ctx"}, {context.Background(), nilLimiter, 0, "limiter"}, {context.Background(), zero, 0, "limiter"}, {context.Background(), l, 0, "n"}, {context.Background(), l, 8, "n"}}
	for _, tc := range casesCall {
		err := tc.l.Wait(tc.ctx, tc.n)
		if !errors.Is(err, ErrInvalidCall) || !strings.Contains(err.Error(), tc.field) {
			t.Fatal(err)
		}
	}
	if l.tokens != 7 || l.head != nil {
		t.Fatal("invalid calls changed state")
	}
}
func TestIntegerCreditAndFraction(t *testing.T) {
	l := newTestLimiter(t, Config{3, 3})
	if !l.Allow(3) {
		t.Fatal("initial burst")
	}
	at := l.anchor.at
	l.refill(at.Add(333333333 * time.Nanosecond))
	if l.tokens != 0 || l.delay(1) != time.Nanosecond {
		t.Fatal(l.tokens, l.delay(1))
	}
	l.refill(at.Add(333333334 * time.Nanosecond))
	if l.tokens != 1 {
		t.Fatal(l.tokens)
	}
	l.tokens--
	l.refill(at.Add(666666666 * time.Nanosecond))
	if l.tokens != 0 || l.delay(1) != time.Nanosecond {
		t.Fatal(l.tokens, l.delay(1))
	}
	l.refill(at.Add(666666667 * time.Nanosecond))
	if l.tokens != 1 {
		t.Fatal(l.tokens)
	}
	l.tokens--
	l.refill(at.Add(time.Second))
	if l.tokens != 1 {
		t.Fatal("fraction lost", l.tokens)
	}
	before := l.anchor.at
	l.refill(before.Add(-time.Second))
	if l.anchor.at != before || l.tokens != 1 {
		t.Fatal("clock rewound")
	}
	l.refill(at.Add(24 * time.Hour))
	if l.tokens != 3 || l.anchor.phase.Sign() != 0 {
		t.Fatal(l.tokens)
	}
}
func TestInitialBurstAndNoFullCredit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newTestLimiter(t, Config{1, 3})
		time.Sleep(time.Hour)
		if !l.Allow(3) || l.Allow(1) {
			t.Fatal("initial capacity")
		}
		started := time.Now()
		if err := l.Wait(context.Background(), 1); err != nil || time.Since(started) != time.Second {
			t.Fatal(err, time.Since(started))
		}
		other := newTestLimiter(t, Config{1, 3})
		if !other.Allow(3) {
			t.Fatal("shared instance budget")
		}
	})
}
func TestFIFOHeadOfLineAndSingleTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newTestLimiter(t, Config{1, 3})
		l.Allow(2)
		first := startWait(l, context.Background(), 3)
		pending(t, l, 1)
		timer := timerOf(l)
		if timer == nil {
			t.Fatal("missing head timer")
		}
		second := startWait(l, context.Background(), 1)
		pending(t, l, 2)
		third := startWait(l, context.Background(), 1)
		nodes := pending(t, l, 3)
		if l.Allow(1) {
			t.Fatal("Allow bypassed head")
		}
		for i, w := range nodes {
			select {
			case <-w.ready:
				if i != 0 {
					t.Fatal("nonhead notified")
				}
			default:
				if i == 0 {
					t.Fatal("head not notified")
				}
			}
		}
		start := time.Now()
		time.Sleep(2 * time.Second)
		if err := <-first; err != nil {
			t.Fatal(err)
		}
		pending(t, l, 2)
		if timerOf(l) != timer {
			t.Fatal("timer not reused")
		}
		time.Sleep(time.Second)
		if err := <-second; err != nil {
			t.Fatal(err)
		}
		pending(t, l, 1)
		if timerOf(l) != timer {
			t.Fatal("second timer allocated")
		}
		time.Sleep(time.Second)
		if err := <-third; err != nil {
			t.Fatal(err)
		}
		pending(t, l, 0)
		if time.Since(start) != 4*time.Second || l.timer != nil || l.tokens != 0 {
			t.Fatal("consumption/timer cleanup", l.tokens)
		}
	})
}
func TestQueueCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newTestLimiter(t, Config{1, 3})
		l.Allow(2)
		cause := errors.New("head canceled")
		headCtx, cancelHead := context.WithCancelCause(context.Background())
		first := startWait(l, headCtx, 3)
		pending(t, l, 1)
		middleCtx, cancelMiddle := context.WithCancel(context.Background())
		middle := startWait(l, middleCtx, 2)
		pending(t, l, 2)
		tailCtx, cancelTail := context.WithCancel(context.Background())
		tail := startWait(l, tailCtx, 2)
		nodes := pending(t, l, 3)
		cancelMiddle()
		if !errors.Is(<-middle, context.Canceled) {
			t.Fatal("middle cancel")
		}
		pending(t, l, 2)
		cancelTail()
		if !errors.Is(<-tail, context.Canceled) {
			t.Fatal("tail cancel")
		}
		pending(t, l, 1)
		for _, w := range nodes[1:] {
			if w.prev != nil || w.next != nil {
				t.Fatal("removed node retains queue")
			}
			select {
			case <-w.ready:
				t.Fatal("canceled nonhead notified")
			default:
			}
		}
		next := startWait(l, context.Background(), 1)
		pending(t, l, 2)
		started := time.Now()
		cancelHead(cause)
		err := <-first
		if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Fatal(err)
		}
		if err := <-next; err != nil || !time.Now().Equal(started) {
			t.Fatal("next kept stale head delay", err)
		}
		pending(t, l, 0)
		if l.tokens != 0 || l.timer != nil {
			t.Fatal("canceled waiter consumed", l.tokens)
		}
	})
}
func TestContextAndDeadlineSemantics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newTestLimiter(t, Config{1, 1})
		cause := errors.New("custom cause")
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(cause)
		err := l.Wait(ctx, 1)
		if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || errors.Is(err, ErrInvalidCall) || l.tokens != 1 {
			t.Fatal(err, l.tokens)
		}
		l.Allow(1)
		short, stop := context.WithTimeoutCause(context.Background(), 500*time.Millisecond, cause)
		defer stop()
		start := time.Now()
		err = l.Wait(short, 1)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, cause) || time.Since(start) != 500*time.Millisecond {
			t.Fatal("predicted deadline rejection", err, time.Since(start))
		}
		if l.tokens != 0 || l.head != nil || l.timer != nil {
			t.Fatal("deadline consumed tokens")
		}
		time.Sleep(500 * time.Millisecond)
		exact, expire := context.WithTimeout(context.Background(), time.Second)
		defer expire()
		l.Allow(1)
		err = l.Wait(exact, 1)
		if !errors.Is(err, context.DeadlineExceeded) || l.tokens != 0 {
			t.Fatal("admitted at deadline", err, l.tokens)
		}
		l2 := newTestLimiter(t, Config{1, 1})
		active, done := context.WithCancel(context.Background())
		err = l2.Wait(active, 1)
		done()
		if err != nil || l2.Allow(1) {
			t.Fatal("admission refunded", err)
		}
	})
}
func TestExtremeRatesBurstAndLongDelay(t *testing.T) {
	if strconv.IntSize == 64 {
		large := uint64(1) << 53
		l := newTestLimiter(t, Config{math.SmallestNonzeroFloat64, math.MaxInt})
		n := int(large) + 1
		if !l.Allow(n) || l.tokens != math.MaxInt-n {
			t.Fatal("rounded weight", l.tokens)
		}
		remaining := l.tokens
		if !l.Allow(remaining) || l.tokens != 0 || l.Allow(1) {
			t.Fatal("rounded capacity")
		}
	}
	synctest.Test(t, func(t *testing.T) {
		fast := newTestLimiter(t, Config{math.MaxFloat64, 2})
		fast.Allow(2)
		if fast.Allow(1) {
			t.Fatal("created tokens without time")
		}
		start := time.Now()
		if err := fast.Wait(context.Background(), 1); err != nil || time.Since(start) != time.Nanosecond {
			t.Fatal(err, time.Since(start))
		}
		slow := newTestLimiter(t, Config{math.SmallestNonzeroFloat64, 1})
		slow.Allow(1)
		if slow.delay(1) != math.MaxInt64 {
			t.Fatal("long delay overflow")
		}
		ctx, cancel := context.WithCancel(context.Background())
		pendingWait := startWait(slow, ctx, 1)
		pending(t, slow, 1)
		cancel()
		if !errors.Is(<-pendingWait, context.Canceled) {
			t.Fatal("uncancelable long delay")
		}
	})
	l := newTestLimiter(t, Config{1e-10, 1})
	l.tokens = 0
	base := l.anchor.at
	l.refill(base.Add(time.Duration(math.MaxInt64)).Add(time.Duration(math.MaxInt64)))
	if l.tokens != 1 {
		t.Fatal("elapsed saturation lost replenishment", l.tokens)
	}
	l = newTestLimiter(t, Config{math.SmallestNonzeroFloat64, 1})
	l.tokens = 0
	base = l.anchor.at
	l.refill(base.Add(time.Duration(math.MaxInt64)).Add(time.Duration(math.MaxInt64)))
	if l.tokens != 0 || l.delay(1) != math.MaxInt64 {
		t.Fatal("over-admitted extreme elapsed")
	}
}
func TestWaitRoundingAgainstRationalOracle(t *testing.T) {
	for _, rate := range []float64{math.SmallestNonzeroFloat64, 0.1, 3, 1e9, math.MaxFloat64} {
		l := newTestLimiter(t, Config{rate, 7})
		l.tokens = 0
		start := l.anchor.at
		for _, elapsed := range []time.Duration{0, 1, 7, 333333333, 333333334, time.Second} {
			l.refill(start.Add(elapsed))
			if l.tokens == 7 {
				continue
			}
			delay := l.delay(7)
			credit := new(big.Rat).Mul(new(big.Rat).SetFloat64(rate), big.NewRat(int64(elapsed), int64(time.Second)))
			missing := new(big.Rat).Sub(big.NewRat(7, 1), credit)
			ns := new(big.Rat).Quo(missing, new(big.Rat).SetFloat64(rate))
			ns.Mul(ns, big.NewRat(int64(time.Second), 1))
			if ns.Cmp(big.NewRat(math.MaxInt64, 1)) > 0 {
				if delay != math.MaxInt64 {
					t.Fatal(delay)
				}
				continue
			}
			actual := big.NewRat(int64(delay), 1)
			if actual.Cmp(ns) < 0 || new(big.Rat).Sub(actual, ns).Cmp(big.NewRat(1, 1)) >= 0 {
				t.Fatal(rate, elapsed, delay, ns)
			}
		}
	}
}
