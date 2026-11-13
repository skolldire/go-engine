package resilience

import (
	"context"
	"errors"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/retry"
)

func TestStressForeignInterfaces(t *testing.T) {
	if os.Getenv("RESILIENCE_STRESS") != "1" {
		t.Skip("set RESILIENCE_STRESS=1")
	}
	var acquired, released, calls atomic.Int64
	cfg := localConfig()
	cfg.Weight = 1
	cfg.Tokens = 1
	f := mustWrap(t, func(context.Context) (int, error) { calls.Add(1); return 7, nil }, cfg, Dependencies{
		Limiter:     &foreignLimiter{func(context.Context, int64) (func(), error) { acquired.Add(1); return func() { released.Add(1) }, nil }},
		RateLimiter: &foreignRate{func(context.Context, int) error { return nil }},
		Breaker:     &foreignBreaker{func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }},
	})
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			for range 500 {
				if value, err := f(context.Background()); err != nil || value != 7 {
					t.Error("changed result")
				}
			}
		})
	}
	wg.Wait()
	if calls.Load() != 12000 || acquired.Load() != 12000 || released.Load() != 12000 {
		t.Fatal(calls.Load(), acquired.Load(), released.Load())
	}
}
func TestNoGoroutineLeaks(t *testing.T) {
	runtime.GC()
	p := pprof.Lookup("goroutineleak")
	if p == nil || p.Count() != 0 {
		t.Fatal("goroutine leak")
	}
}
func FuzzConfig(f *testing.F) {
	f.Add(int64(0), int64(0), int64(0), 0)
	f.Add(int64(-1), int64(1), int64(2), 1)
	f.Fuzz(func(t *testing.T, total, attempt, admission int64, retries int) {
		cfg := DefaultConfig()
		cfg.TotalTimeout = time.Duration(total)
		cfg.AttemptTimeout = time.Duration(attempt)
		cfg.AdmissionTimeout = time.Duration(admission)
		cfg.Retry.MaxRetries = retries
		_, err := Wrap(func(context.Context) (int, error) { t.Fatal("constructor executed"); return 0, nil }, cfg, Dependencies{Classify: func(error) retry.Decision { return retry.DecisionStop }})
		valid := total >= 0 && attempt >= 0 && admission >= 0 && retries >= 0
		if (err == nil) != valid {
			t.Fatal("config validation")
		}
	})
}
func FuzzSequences(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 32 {
			return
		}
		for _, event := range data {
			cfg := localConfig()
			cfg.Retry.MaxRetries = 1
			failure := errors.New("original")
			calls := 0
			deps := Dependencies{Classify: func(error) retry.Decision { return retry.DecisionStop }, Breaker: &foreignBreaker{func(ctx context.Context, fn func(context.Context) error) error {
				if event%3 == 0 {
					return failure
				}
				return fn(ctx)
			}}}
			op := mustWrap(t, func(context.Context) (int, error) {
				calls++
				if event%3 == 1 {
					return 17, failure
				}
				return 17, nil
			}, cfg, deps)
			value, err := op(context.Background())
			switch event % 3 {
			case 0:
				if calls != 0 || value != 0 || !errors.Is(err, ErrRejected) || !errors.Is(err, failure) {
					t.Fatal("rejection")
				}
			case 1:
				if calls != 1 || value != 17 || err != failure || errors.Is(err, ErrRejected) {
					t.Fatal("operation error")
				}
			case 2:
				if calls != 1 || value != 17 || err != nil {
					t.Fatal("success")
				}
			}
		}
	})
}
func FuzzBudgets(f *testing.F) {
	f.Add(uint8(0), uint8(0))
	f.Add(uint8(1), uint8(2))
	f.Fuzz(func(t *testing.T, total, attempt uint8) {
		synctest.Test(t, func(t *testing.T) {
			cfg := localConfig()
			cfg.TotalTimeout = time.Duration(total%4) * time.Millisecond
			cfg.AttemptTimeout = time.Duration(attempt%4) * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
			defer cancel()
			op := mustWrap(t, func(ctx context.Context) (int, error) { <-ctx.Done(); return 17, ctx.Err() }, cfg, Dependencies{})
			value, err := op(ctx)
			if value != 17 || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("budget result")
			}
		})
	})
}
func BenchmarkWrapExecution(b *testing.B) {
	cfg := localConfig()
	cfg.Weight = 1
	cfg.Tokens = 1
	op := mustWrap(b, func(context.Context) (int, error) { return 7, nil }, cfg, Dependencies{Limiter: &foreignLimiter{func(context.Context, int64) (func(), error) { return func() {}, nil }}, RateLimiter: &foreignRate{func(context.Context, int) error { return nil }}, Breaker: &foreignBreaker{func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }}})
	b.ReportAllocs()
	for b.Loop() {
		if value, err := op(context.Background()); value != 7 || err != nil {
			b.Fatal("fixture")
		}
	}
}
