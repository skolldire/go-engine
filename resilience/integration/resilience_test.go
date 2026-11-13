package integration

import (
	"context"
	"errors"
	"math"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/resilience"

	"github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/concurrency"
	"github.com/skolldire/go-engine/ratelimit"
	"github.com/skolldire/go-engine/retry"
)

type domainError struct{ code int }

func (e *domainError) Error() string { return "domain" }
func retryAll(error) retry.Decision  { return retry.DecisionRetry }
func config() resilience.Config {
	cfg := resilience.DefaultConfig()
	cfg.Retry.InitialBackoff = time.Nanosecond
	cfg.Retry.MaxBackoff = time.Nanosecond
	cfg.Retry.Jitter = retry.JitterNone
	return cfg
}
func limiter(t *testing.T, capacity int64) *concurrency.Limiter {
	t.Helper()
	l, err := concurrency.NewLimiter(concurrency.LimiterConfig{Capacity: capacity})
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func rate(t *testing.T, r float64, burst int) *ratelimit.Limiter {
	t.Helper()
	l, err := ratelimit.New(ratelimit.Config{RatePerSecond: r, Burst: burst})
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func breaker(t *testing.T, threshold uint64, classify func(error) bool) *circuitbreaker.Breaker {
	t.Helper()
	cfg := circuitbreaker.DefaultConfig()
	cfg.Name = "memory"
	cfg.RequestThreshold = threshold
	b, err := circuitbreaker.New(cfg, circuitbreaker.Dependencies{Classify: classify})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func wrap[T any](t *testing.T, op client.Operation[T], cfg resilience.Config, deps resilience.Dependencies) client.Operation[T] {
	t.Helper()
	fn, err := resilience.Wrap(op, cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	return fn
}
func acquireNow(t *testing.T, l resilience.Limiter) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	release, err := l.Acquire(ctx, 1)
	if err != nil {
		t.Fatal("permit not available", err)
	}
	return release
}
func TestConfigurationAndValidation(t *testing.T) {
	want := resilience.Config{Order: resilience.OrderRetryThenBreaker, Retry: retry.DefaultConfig()}
	if resilience.DefaultConfig() != want || want.Validate() != nil {
		t.Fatal(resilience.DefaultConfig())
	}
	cases := []struct {
		field  string
		change func(*resilience.Config)
	}{
		{"Order", func(c *resilience.Config) { c.Order = "secret-order" }},
		{"TotalTimeout", func(c *resilience.Config) { c.TotalTimeout = -1 }},
		{"AttemptTimeout", func(c *resilience.Config) { c.AttemptTimeout = -1 }},
		{"AdmissionTimeout", func(c *resilience.Config) { c.AdmissionTimeout = -1 }},
		{"Retry.InitialBackoff", func(c *resilience.Config) { c.Retry.InitialBackoff = 0 }},
		{"Retry.TotalTimeout", func(c *resilience.Config) { c.Retry.TotalTimeout = time.Second }},
		{"Weight", func(c *resilience.Config) { c.Weight = -1 }},
		{"Tokens", func(c *resilience.Config) { c.Tokens = -1 }},
	}
	for _, tc := range cases {
		cfg := want
		tc.change(&cfg)
		fn, err := resilience.Wrap(func(context.Context) (int, error) { t.Fatal("construction I/O"); return 0, nil }, cfg, resilience.Dependencies{})
		var ce *resilience.ConfigError
		if fn != nil || !errors.Is(err, resilience.ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tc.field || strings.Contains(err.Error(), "secret-order") {
			t.Fatal(err)
		}
	}
	if err := (resilience.Config{}).Validate(); !errors.Is(err, resilience.ErrInvalidConfig) || !errors.Is(err, retry.ErrInvalidConfig) {
		t.Fatal(err)
	}
	_, err := resilience.Wrap[int](nil, resilience.Config{}, resilience.Dependencies{})
	if !errors.Is(err, resilience.ErrInvalidCall) {
		t.Fatal(err)
	}
	cfg := config()
	cfg.Retry.MaxRetries = 1
	b := breaker(t, 5, func(error) bool { return true })
	l := limiter(t, 1)
	r := rate(t, 1, 1)
	casesDeps := []struct {
		cfg   resilience.Config
		deps  resilience.Dependencies
		field string
	}{
		{func() resilience.Config { c := cfg; c.Order = ""; return c }(), resilience.Dependencies{Breaker: b}, "Order"},
		{cfg, resilience.Dependencies{}, "Classify"},
		{config(), resilience.Dependencies{Limiter: l}, "Weight"},
		{func() resilience.Config { c := config(); c.Weight = 1; return c }(), resilience.Dependencies{}, "Weight"},
		{config(), resilience.Dependencies{RateLimiter: r}, "Tokens"},
		{func() resilience.Config { c := config(); c.Tokens = 1; return c }(), resilience.Dependencies{}, "Tokens"},
	}
	for _, tc := range casesDeps {
		_, err := resilience.Wrap(func(context.Context) (int, error) { return 1, nil }, tc.cfg, tc.deps)
		var ce *resilience.ConfigError
		if !errors.As(err, &ce) || ce.Field != tc.field {
			t.Fatal(err)
		}
	}
	emptyOrder := config()
	emptyOrder.Order = ""
	if _, err := resilience.Wrap(func(context.Context) (int, error) { return 1, nil }, emptyOrder, resilience.Dependencies{}); err != nil {
		t.Fatal(err)
	}
}
func TestPureCaptureAndCombinations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for mask := 0; mask < 16; mask++ {
			cfg := config()
			deps := resilience.Dependencies{}
			calls := 0
			if mask&1 != 0 {
				cfg.Retry.MaxRetries = 1
				deps.Classify = retryAll
			}
			if mask&2 != 0 {
				deps.Breaker = breaker(t, 5, func(error) bool { return true })
			}
			if mask&4 != 0 {
				cfg.Weight = 1
				deps.Limiter = limiter(t, 1)
			}
			if mask&8 != 0 {
				cfg.Tokens = 1
				deps.RateLimiter = rate(t, 1, 2)
			}
			before := cfg
			fn := wrap(t, func(ctx context.Context) (int, error) { calls++; return 42, nil }, cfg, deps)
			if calls != 0 || cfg != before {
				t.Fatal("impure Wrap")
			}
			if deps.Breaker != nil && deps.Breaker.(*circuitbreaker.Breaker).Stats().Requests != 0 {
				t.Fatal("breaker touched")
			}
			if deps.Limiter != nil {
				acquireNow(t, deps.Limiter)()
			}
			if deps.RateLimiter != nil && !deps.RateLimiter.(*ratelimit.Limiter).Allow(1) {
				t.Fatal("rate touched during Wrap")
			}
			cfg.Retry.MaxRetries = 99
			cfg.Retry.InitialBackoff = -1
			cfg.Tokens = 99
			cfg.Weight = 99
			value, err := fn(context.Background())
			if value != 42 || err != nil || calls != 1 {
				t.Fatal(mask, value, err, calls)
			}
		}
		cfg := config()
		cfg.TotalTimeout = time.Second
		fn := wrap(t, func(context.Context) (int, error) { return 7, nil }, cfg, resilience.Dependencies{})
		time.Sleep(time.Hour)
		if value, err := fn(context.Background()); value != 7 || err != nil {
			t.Fatal("budget started during Wrap", value, err)
		}
	})
}
func TestContextValuesErrorsAndRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		type key struct{}
		ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), key{}, "v"), time.Hour)
		defer cancel()
		original := &domainError{7}
		payload := map[string]int{"value": 42}
		fn := wrap(t, func(received context.Context) (map[string]int, error) {
			if received != ctx || received.Value(key{}) != "v" {
				t.Fatal("context changed")
			}
			return payload, original
		}, config(), resilience.Dependencies{})
		got, err := fn(ctx)
		if err != original || got["value"] != 42 {
			t.Fatal(got, err)
		}
		got["value"] = 10
		if payload["value"] != 10 {
			t.Fatal("result copied")
		}
		var typed *domainError
		if !errors.As(err, &typed) || typed != original {
			t.Fatal(err)
		}
		if value, err := fn(nil); value != nil || !errors.Is(err, resilience.ErrInvalidCall) {
			t.Fatal(value, err)
		}
		cause := errors.New("caller cause")
		canceled, stop := context.WithCancelCause(ctx)
		stop(cause)
		if value, err := fn(canceled); value != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Fatal(value, err)
		}
		for _, decision := range []retry.Decision{retry.DecisionStop, retry.DecisionRetry, retry.DecisionReconcile, "invalid"} {
			cfg := config()
			cfg.Retry.MaxRetries = 2
			calls, classifies := 0, 0
			f := wrap(t, func(context.Context) (int, error) { calls++; return 20 + calls, original }, cfg, resilience.Dependencies{Classify: func(err error) retry.Decision {
				classifies++
				if err != original {
					t.Fatal(err)
				}
				return decision
			}})
			value, err := f(ctx)
			expected := 1
			if decision == retry.DecisionRetry {
				expected = 3
			}
			if calls != expected || classifies != expected || value != 20+expected || !errors.Is(err, original) {
				t.Fatal(value, err, calls, classifies)
			}
			if decision == retry.DecisionStop || decision == retry.DecisionRetry {
				if err != original {
					t.Fatal("error identity")
				}
			}
			if decision == retry.DecisionReconcile && !errors.Is(err, retry.ErrReconciliationRequired) {
				t.Fatal(err)
			}
		}
		classified := 0
		f := wrap(t, func(context.Context) (int, error) { return 3, original }, config(), resilience.Dependencies{Classify: func(error) retry.Decision { classified++; return retry.DecisionReconcile }})
		value, err := f(ctx)
		if value != 3 || classified != 1 || !errors.Is(err, retry.ErrReconciliationRequired) {
			t.Fatal(value, err)
		}
	})
}
func TestOrderAndPermitBeforeRetryClassification(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var order []string
		original := errors.New("remote")
		l := limiter(t, 1)
		r := rate(t, 1, 2)
		b := breaker(t, 5, func(err error) bool { order = append(order, "breaker.classify"); return errors.Is(err, original) })
		cfg := config()
		cfg.Retry.MaxRetries = 1
		cfg.Weight = 1
		cfg.Tokens = 1
		calls := 0
		fn := wrap(t, func(context.Context) (int, error) {
			calls++
			order = append(order, "io")
			if b.Stats().InFlight != 1 {
				t.Error("not breaker-admitted")
			}
			if calls == 1 {
				return 7, original
			}
			return 42, nil
		}, cfg, resilience.Dependencies{Limiter: l, RateLimiter: r, Breaker: b, Classify: func(error) retry.Decision {
			order = append(order, "retry.classify")
			acquireNow(t, l)()
			return retry.DecisionRetry
		}})
		value, err := fn(context.Background())
		if value != 42 || err != nil || calls != 2 {
			t.Fatal(value, err, calls)
		}
		if !reflect.DeepEqual(order, []string{"io", "breaker.classify", "retry.classify", "io"}) {
			t.Fatal(order)
		}
		if r.Allow(1) || b.Stats().Failures != 1 || b.Stats().Successes != 1 {
			t.Fatal("attempt accounting", b.Stats())
		}
		acquireNow(t, l)()
	})
}
func TestAdmissionBeforeProbeAndAttemptClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := limiter(t, 1)
		r := rate(t, 1, 1)
		r.Allow(1)
		b := breaker(t, 1, func(error) bool { return true })
		circuitbreaker.Execute(context.Background(), b, func(context.Context) (int, error) { return 0, errors.New("trip") })
		time.Sleep(30 * time.Second)
		r.Allow(1)
		cfg := config()
		cfg.Weight = 1
		cfg.Tokens = 1
		cfg.AttemptTimeout = 100 * time.Millisecond
		calls := 0
		fn := wrap(t, func(ctx context.Context) (int, error) {
			calls++
			deadline, _ := ctx.Deadline()
			if time.Until(deadline) != cfg.AttemptTimeout {
				t.Error("attempt clock started in queue")
			}
			return 42, nil
		}, cfg, resilience.Dependencies{Limiter: l, RateLimiter: r, Breaker: b})
		done := make(chan error, 1)
		go func() { _, err := fn(context.Background()); done <- err }()
		synctest.Wait()
		if b.Stats().InFlight != 0 {
			t.Fatal("probe held waiting for rate")
		}
		release := acquireNow(t, l)
		time.Sleep(time.Second)
		synctest.Wait()
		if b.Stats().InFlight != 0 || calls != 0 {
			t.Fatal("probe/attempt held waiting concurrency")
		}
		release()
		if err := <-done; err != nil || calls != 1 {
			t.Fatal(err, calls)
		}
		if b.State() != circuitbreaker.StateClosed {
			t.Fatal(b.Stats())
		}
	})
}
func TestAdmissionJointBudgetAndPermanentErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := limiter(t, 1)
		held := acquireNow(t, l)
		defer held()
		r := rate(t, 2, 1)
		r.Allow(1)
		b := breaker(t, 5, func(error) bool { t.Error("local failure classified"); return true })
		cfg := config()
		cfg.Weight = 1
		cfg.Tokens = 1
		cfg.AdmissionTimeout = time.Second
		cfg.Retry.MaxRetries = 4
		classified := 0
		fn := wrap(t, func(context.Context) (int, error) { t.Error("admission failure reached I/O"); return 1, nil }, cfg, resilience.Dependencies{Limiter: l, RateLimiter: r, Breaker: b, Classify: func(error) retry.Decision { classified++; return retry.DecisionRetry }})
		started := time.Now()
		_, err := fn(context.Background())
		if time.Since(started) != time.Second || !errors.Is(err, resilience.ErrAdmission) || !errors.Is(err, resilience.ErrAdmissionTimeout) || !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "concurrency") || classified != 0 {
			t.Fatal(err, time.Since(started), classified)
		}
		if b.Stats().Requests != 0 {
			t.Fatal(b.Stats())
		}
		for _, which := range []string{"concurrency", "ratelimit"} {
			c := config()
			d := resilience.Dependencies{Classify: func(error) retry.Decision { t.Error("permanent failure retried"); return retry.DecisionRetry }}
			c.Retry.MaxRetries = 2
			expected := error(nil)
			if which == "concurrency" {
				d.Limiter = limiter(t, 1)
				c.Weight = 2
				expected = concurrency.ErrJobTooLarge
			} else {
				d.RateLimiter = rate(t, 1, 1)
				c.Tokens = 2
				expected = ratelimit.ErrInvalidCall
			}
			f := wrap(t, func(context.Context) (int, error) { t.Error("invalid weight I/O"); return 0, nil }, c, d)
			_, err := f(context.Background())
			if !errors.Is(err, resilience.ErrAdmission) || !errors.Is(err, expected) || !strings.Contains(err.Error(), which) {
				t.Fatal(err)
			}
		}
	})
}
func TestBudgetsCausesAndErrorPrecedence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, which := range []string{"total", "attempt", "parent"} {
			cfg := config()
			cfg.TotalTimeout = 2 * time.Second
			cfg.AttemptTimeout = time.Second
			parent, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			cause := errors.New("parent deadline")
			if which == "total" {
				cfg.TotalTimeout = 500 * time.Millisecond
			}
			if which == "parent" {
				var stop context.CancelFunc
				parent, stop = context.WithTimeoutCause(parent, 100*time.Millisecond, cause)
				defer stop()
			}
			expected := resilience.ErrAttemptTimeout
			duration := time.Second
			if which == "total" {
				expected = resilience.ErrTotalTimeout
				duration = 500 * time.Millisecond
			}
			if which == "parent" {
				expected = cause
				duration = 100 * time.Millisecond
			}
			b := breaker(t, 5, func(err error) bool {
				if !errors.Is(err, expected) {
					t.Error("cause absent before breaker classification", err)
				}
				return false
			})
			cfg.Order = resilience.OrderRetryThenBreaker
			f := wrap(t, func(ctx context.Context) (int, error) { <-ctx.Done(); return 17, ctx.Err() }, cfg, resilience.Dependencies{Breaker: b})
			started := time.Now()
			value, err := f(parent)
			if value != 17 || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, expected) || time.Since(started) != duration {
				t.Fatal(which, value, err, time.Since(started))
			}
			if which != "attempt" && errors.Is(err, resilience.ErrAttemptTimeout) || which != "total" && errors.Is(err, resilience.ErrTotalTimeout) {
				t.Fatal("wrong budget attribution", which, err)
			}
		}
		original := &domainError{9}
		for _, success := range []bool{false, true} {
			cfg := config()
			cfg.TotalTimeout = time.Second
			cfg.AttemptTimeout = 100 * time.Millisecond
			f := wrap(t, func(ctx context.Context) (int, error) {
				<-ctx.Done()
				if success {
					return 42, nil
				}
				return 7, original
			}, cfg, resilience.Dependencies{})
			value, err := f(context.Background())
			if success {
				if value != 42 || err != nil {
					t.Fatal(value, err)
				}
			} else if value != 7 || err != original {
				t.Fatal("business error replaced", value, err)
			}
		}
		cfg := config()
		cfg.TotalTimeout = time.Second
		cfg.Retry.InitialBackoff = time.Second
		cfg.Retry.MaxBackoff = time.Second
		cfg.Retry.MaxRetries = 2
		calls := 0
		f := wrap(t, func(context.Context) (int, error) { calls++; return 11, original }, cfg, resilience.Dependencies{Classify: retryAll})
		value, err := f(context.Background())
		if value != 11 || calls != 1 || !errors.Is(err, resilience.ErrTotalTimeout) || !errors.Is(err, original) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("backoff total cause", value, err, calls)
		}
		cfg = config()
		cfg.TotalTimeout = time.Second
		cfg.Retry.MaxRetries = 2
		f = wrap(t, func(context.Context) (int, error) { return 31, original }, cfg, resilience.Dependencies{Classify: func(error) retry.Decision { time.Sleep(2 * time.Second); return retry.DecisionReconcile }})
		value, err = f(context.Background())
		if value != 31 || !errors.Is(err, retry.ErrReconciliationRequired) || !errors.Is(err, original) || errors.Is(err, resilience.ErrTotalTimeout) {
			t.Fatal("uncertainty hidden", value, err)
		}
	})
}
func TestAdmissionParentCauseAndContextOwnership(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, which := range []string{"rate", "concurrency"} {
			for _, own := range []bool{false, true} {
				cfg := config()
				cfg.AdmissionTimeout = 2 * time.Second
				deps := resilience.Dependencies{}
				if which == "rate" {
					cfg.Tokens = 1
					deps.RateLimiter = rate(t, 1, 1)
					deps.RateLimiter.(*ratelimit.Limiter).Allow(1)
				} else {
					cfg.Weight = 1
					deps.Limiter = limiter(t, 1)
					held := acquireNow(t, deps.Limiter)
					defer held()
				}
				ctx := context.Background()
				cause := errors.New("caller budget")
				if own {
					cfg.TotalTimeout = 100 * time.Millisecond
				} else {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeoutCause(ctx, 100*time.Millisecond, cause)
					defer cancel()
				}
				f := wrap(t, func(context.Context) (int, error) { t.Error("expired admission entered I/O"); return 0, nil }, cfg, deps)
				_, err := f(ctx)
				expected := cause
				if own {
					expected = resilience.ErrTotalTimeout
				}
				if !errors.Is(err, expected) || !errors.Is(err, resilience.ErrAdmission) || errors.Is(err, resilience.ErrAdmissionTimeout) {
					t.Fatal(which, own, err)
				}
			}
		}
		for _, mask := range []int{0, 1, 2, 3} {
			cfg := config()
			cfg.AdmissionTimeout = time.Nanosecond
			cfg.Weight = 1
			if mask&1 != 0 {
				cfg.TotalTimeout = time.Hour
			}
			if mask&2 != 0 {
				cfg.AttemptTimeout = time.Hour
			}
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			l := limiter(t, 1)
			f := wrap(t, func(ctx context.Context) (context.Context, error) {
				if ctx.Err() != nil {
					t.Error("admission context passed to I/O")
				}
				return ctx, nil
			}, cfg, resilience.Dependencies{Limiter: l})
			linked, err := f(parent)
			if err != nil || parent.Err() != nil {
				t.Fatal(err)
			}
			if mask == 0 {
				if linked != parent || linked.Err() != nil {
					t.Fatal("parent context replaced/canceled")
				}
			} else if linked.Err() != context.Canceled {
				t.Fatal("derived streaming context not canceled")
			}
			acquireNow(t, l)()
		}
	})
}
func TestLocalStopsAndBreakerPerAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, probe := range []bool{false, true} {
			b := breaker(t, 1, func(error) bool { return true })
			circuitbreaker.Execute(context.Background(), b, func(context.Context) (int, error) { return 0, errors.New("remote") })
			var gate chan struct{}
			var finished chan struct{}
			if probe {
				time.Sleep(30 * time.Second)
				gate = make(chan struct{})
				finished = make(chan struct{})
				go func() {
					defer close(finished)
					circuitbreaker.Execute(context.Background(), b, func(context.Context) (int, error) { <-gate; return 1, nil })
				}()
				synctest.Wait()
			}
			cfg := config()
			cfg.Retry.MaxRetries = 3
			cfg.Weight = 1
			cfg.Tokens = 1
			l := limiter(t, 1)
			r := rate(t, math.SmallestNonzeroFloat64, 1)
			f := wrap(t, func(context.Context) (int, error) { t.Error("rejected I/O"); return 9, nil }, cfg, resilience.Dependencies{Breaker: b, Limiter: l, RateLimiter: r, Classify: func(error) retry.Decision {
				t.Error("local rejection classified by caller")
				return retry.DecisionRetry
			}})
			value, err := f(context.Background())
			expected := circuitbreaker.ErrOpen
			if probe {
				expected = circuitbreaker.ErrProbeLimit
			}
			if value != 0 || !errors.Is(err, expected) || !errors.Is(err, resilience.ErrRejected) || r.Allow(1) {
				t.Fatal(value, err)
			}
			acquireNow(t, l)()
			if probe {
				close(gate)
				<-finished
			}
		}
		b := breaker(t, 1, func(error) bool { return true })
		cfg := config()
		cfg.Retry.MaxRetries = 3
		calls := 0
		f := wrap(t, func(context.Context) (int, error) { calls++; return 2, errors.New("remote") }, cfg, resilience.Dependencies{Breaker: b, Classify: retryAll})
		value, err := f(context.Background())
		if calls != 1 || value != 0 || (!errors.Is(err, circuitbreaker.ErrOpen) || !errors.Is(err, resilience.ErrRejected)) {
			t.Fatal("breaker saw aggregate", calls, value, err)
		}
	})
}
func catchPanic(f func()) (value any) { defer func() { value = recover() }(); f(); return nil }
func TestAbnormalReleaseAndNonCooperation(t *testing.T) {
	for _, where := range []string{"operation", "breaker classifier", "retry classifier"} {
		for _, exit := range []bool{false, true} {
			synctest.Test(t, func(t *testing.T) {
				l := limiter(t, 1)
				marker := &domainError{11}
				abort := func() {
					if exit {
						runtime.Goexit()
					}
					panic(marker)
				}
				cfg := config()
				cfg.Weight = 1
				cfg.TotalTimeout = time.Hour
				cfg.AttemptTimeout = time.Minute
				deps := resilience.Dependencies{Limiter: l, Breaker: breaker(t, 5, func(error) bool {
					if where == "breaker classifier" {
						abort()
					}
					return true
				}), Classify: func(error) retry.Decision {
					acquireNow(t, l)()
					if where == "retry classifier" {
						abort()
					}
					return retry.DecisionStop
				}}
				f := wrap(t, func(context.Context) (int, error) {
					if where == "operation" {
						abort()
					}
					return 3, marker
				}, cfg, deps)
				done := make(chan struct{})
				go func() {
					defer close(done)
					work := func() { f(context.Background()) }
					if exit {
						work()
						t.Error("Goexit returned")
					} else if catchPanic(work) != marker {
						t.Error("panic changed")
					}
				}()
				<-done
				acquireNow(t, l)()
				c := deps.Breaker.(*circuitbreaker.Breaker).Stats()
				if c.InFlight != 0 || where != "retry classifier" && c.Panics != 1 {
					t.Fatal(c)
				}
			})
		}
	}
	synctest.Test(t, func(t *testing.T) {
		l := limiter(t, 1)
		b := breaker(t, 5, func(error) bool { return false })
		cfg := config()
		cfg.Weight = 1
		cfg.AttemptTimeout = time.Second
		gate := make(chan struct{})
		done := make(chan struct{})
		f := wrap(t, func(context.Context) (int, error) { <-gate; return 42, nil }, cfg, resilience.Dependencies{Limiter: l, Breaker: b})
		go func() {
			defer close(done)
			value, err := f(context.Background())
			if value != 42 || err != nil {
				t.Error(value, err)
			}
		}()
		synctest.Wait()
		time.Sleep(2 * time.Second)
		if b.Stats().InFlight != 1 {
			t.Fatal("noncooperative attempt abandoned", b.Stats())
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		if release, err := l.Acquire(ctx, 1); err == nil {
			release()
			t.Fatal("noncooperative permit released early")
		}
		select {
		case <-done:
			t.Fatal("false termination")
		default:
		}
		close(gate)
		<-done
		acquireNow(t, l)()
	})
}
func TestReusableConcurrentWrapper(t *testing.T) {
	l := limiter(t, 4)
	cfg := config()
	cfg.Weight = 1
	f := wrap(t, func(context.Context) (int, error) { return 42, nil }, cfg, resilience.Dependencies{Limiter: l})
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 100 {
				value, err := f(context.Background())
				if value != 42 || err != nil {
					t.Error(value, err)
				}
			}
		})
	}
	wg.Wait()
	release, err := l.Acquire(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

type deadlineError struct{ cause error }

func (e *deadlineError) Error() string { return "I/O deadline" }
func (e *deadlineError) Unwrap() error { return e.cause }
func TestAttemptEnrichmentBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, kind := range []string{"wrapped deadline", "canceled", "early deadline"} {
			cfg := config()
			cfg.AttemptTimeout = time.Second
			original := &deadlineError{context.DeadlineExceeded}
			expected := error(original)
			if kind == "canceled" {
				expected = context.Canceled
			}
			b := breaker(t, 5, func(err error) bool {
				if kind == "wrapped deadline" {
					var typed *deadlineError
					if !errors.Is(err, resilience.ErrAttemptTimeout) || !errors.As(err, &typed) || typed != original {
						t.Error("classification lost typed cause", err)
					}
				} else if err != expected {
					t.Error("unrelated error enriched", err)
				}
				return false
			})
			f := wrap(t, func(ctx context.Context) (int, error) {
				if kind != "early deadline" {
					<-ctx.Done()
				}
				return 21, expected
			}, cfg, resilience.Dependencies{Breaker: b})
			value, err := f(context.Background())
			if value != 21 {
				t.Fatal(value)
			}
			if kind == "wrapped deadline" {
				if !errors.Is(err, original) || !errors.Is(err, resilience.ErrAttemptTimeout) {
					t.Fatal(err)
				}
			} else if err != expected {
				t.Fatal("identity lost", kind, err)
			}
		}
	})
}
