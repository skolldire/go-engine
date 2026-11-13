package retry

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type domainError struct{ code int }

func (e *domainError) Error() string { return "domain failure" }
func retryAll(error) Decision        { return DecisionRetry }
func fastConfig() Config {
	cfg := DefaultConfig()
	cfg.InitialBackoff = time.Nanosecond
	cfg.MaxBackoff = 8 * time.Nanosecond
	cfg.Jitter = JitterNone
	return cfg
}

func TestConfig(t *testing.T) {
	want := Config{InitialBackoff: 100 * time.Millisecond, MaxBackoff: 10 * time.Second, Multiplier: 2, Jitter: JitterFull}
	if got := DefaultConfig(); got != want || got.Validate() != nil {
		t.Fatal(got)
	}
	cases := []struct {
		name, field string
		edit        func(*Config)
	}{
		{"retries", "MaxRetries", func(c *Config) { c.MaxRetries = -1 }},
		{"initial", "InitialBackoff", func(c *Config) { c.InitialBackoff = 0 }},
		{"max", "MaxBackoff", func(c *Config) { c.MaxBackoff = c.InitialBackoff - 1 }},
		{"timeout", "TotalTimeout", func(c *Config) { c.TotalTimeout = -1 }},
		{"multiplier", "Multiplier", func(c *Config) { c.Multiplier = 0 }},
		{"nan", "Multiplier", func(c *Config) { c.Multiplier = math.NaN() }},
		{"inf", "Multiplier", func(c *Config) { c.Multiplier = math.Inf(1) }},
		{"negative inf", "Multiplier", func(c *Config) { c.Multiplier = math.Inf(-1) }},
		{"jitter", "Jitter", func(c *Config) { c.Jitter = "sensitive-invalid" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := want
			tc.edit(&cfg)
			err := cfg.Validate()
			var ce *ConfigError
			if !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tc.field || strings.Contains(err.Error(), "sensitive-invalid") {
				t.Fatal(err)
			}
		})
	}
	if err := (Config{}).Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	cfg := want
	cfg.MaxRetries = math.MaxInt
	cfg.InitialBackoff = math.MaxInt64
	cfg.MaxBackoff = math.MaxInt64
	cfg.Multiplier = math.MaxFloat64
	cfg.TotalTimeout = math.MaxInt64
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidationOrder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxRetries = 1
	cases := []struct {
		ctx      context.Context
		cfg      Config
		op       func(context.Context) (int, error)
		class    Classifier
		sentinel error
		field    string
	}{
		{nil, Config{}, nil, nil, ErrInvalidCall, "ctx"},
		{context.Background(), Config{}, nil, nil, ErrInvalidConfig, "InitialBackoff"},
		{context.Background(), cfg, nil, nil, ErrInvalidCall, "operation"},
		{context.Background(), cfg, func(context.Context) (int, error) { t.Fatal("called"); return 1, nil }, nil, ErrInvalidCall, "classify"},
	}
	for _, tc := range cases {
		got, err := Do(tc.ctx, tc.cfg, tc.class, tc.op)
		if got != 0 || !errors.Is(err, tc.sentinel) || !strings.Contains(err.Error(), tc.field) {
			t.Fatal(got, err)
		}
	}
}

func TestOutcomes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := &domainError{42}
		for _, decision := range []Decision{DecisionStop, DecisionRetry, DecisionReconcile, "secret-invalid"} {
			for _, retries := range []int{0, 1, 4} {
				cfg := fastConfig()
				cfg.MaxRetries = retries
				before := cfg
				calls, classified := 0, 0
				got, err := Do(context.Background(), cfg, func(e error) Decision {
					classified++
					if e != original {
						t.Fatal(e)
					}
					return decision
				}, func(context.Context) (int, error) { calls++; return calls + 20, original })
				want := 1
				if decision == DecisionRetry {
					want += retries
				}
				if calls != want || classified != want || got != calls+20 || cfg != before || !errors.Is(err, original) {
					t.Fatal(got, err, calls, classified)
				}
				var typed *domainError
				if !errors.As(err, &typed) || typed != original {
					t.Fatal(err)
				}
				switch decision {
				case DecisionStop, DecisionRetry:
					if err != original {
						t.Fatal("identity lost", err)
					}
				case DecisionReconcile:
					if !errors.Is(err, ErrReconciliationRequired) {
						t.Fatal(err)
					}
				default:
					if !errors.Is(err, ErrInvalidDecision) || strings.Contains(err.Error(), string(decision)) {
						t.Fatal(err)
					}
				}
			}
		}
		calls := 0
		cfg := fastConfig()
		cfg.MaxRetries = 3
		got, err := Do(context.Background(), cfg, retryAll, func(context.Context) (string, error) {
			calls++
			if calls < 3 {
				return "partial", original
			}
			return "complete", nil
		})
		if got != "complete" || err != nil || calls != 3 {
			t.Fatal(got, err, calls)
		}
		gotInt, err := Do(context.Background(), DefaultConfig(), nil, func(context.Context) (int, error) { return 5, original })
		if gotInt != 5 || err != original {
			t.Fatal(gotInt, err)
		}
		_, err = Do(context.Background(), DefaultConfig(), func(error) Decision { t.Fatal("classified success"); return DecisionStop }, func(context.Context) (int, error) { return 8, nil })
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestContextAndPrecedence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cause := errors.New("caller cause")
		original := &domainError{3}
		canceled, cancel := context.WithCancelCause(context.Background())
		cancel(cause)
		got, err := Do(canceled, DefaultConfig(), nil, func(context.Context) (int, error) { t.Fatal("pre-canceled call"); return 9, nil })
		if got != 0 || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || errors.Is(err, ErrInvalidCall) {
			t.Fatal(got, err)
		}
		type key struct{}
		parent := context.WithValue(context.Background(), key{}, "value")
		for _, decision := range []Decision{DecisionStop, DecisionReconcile, "bad", DecisionRetry} {
			ctx, cancel := context.WithCancelCause(parent)
			cfg := fastConfig()
			cfg.MaxRetries = 3
			calls := 0
			got, err := Do(ctx, cfg, func(error) Decision { return decision }, func(received context.Context) (int, error) {
				calls++
				if received != ctx || received.Value(key{}) != "value" {
					t.Fatal("context replaced")
				}
				cancel(cause)
				return 13, original
			})
			if got != 13 || calls != 1 || !errors.Is(err, original) {
				t.Fatal(got, err, calls)
			}
			switch decision {
			case DecisionStop:
				if err != original {
					t.Fatal(err)
				}
			case DecisionReconcile:
				if !errors.Is(err, ErrReconciliationRequired) {
					t.Fatal(err)
				}
			case DecisionRetry:
				if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
					t.Fatal(err)
				}
			default:
				if !errors.Is(err, ErrInvalidDecision) {
					t.Fatal(err)
				}
			}
		}
		ctx, done := context.WithCancel(parent)
		got, err = Do(ctx, DefaultConfig(), nil, func(received context.Context) (int, error) { done(); return 21, nil })
		if got != 21 || err != nil {
			t.Fatal(got, err)
		}
		cfg := fastConfig()
		cfg.MaxRetries = 1
		cfg.TotalTimeout = time.Second
		start := time.Now()
		got, err = Do(parent, cfg, retryAll, func(ctx context.Context) (int, error) { <-ctx.Done(); return 7, original })
		if got != 7 || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, original) || time.Since(start) != time.Second {
			t.Fatal(got, err, time.Since(start))
		}
	})
}

func TestBudgetAndContextOwnership(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := errors.New("retryable")
		cfg := DefaultConfig()
		cfg.MaxRetries = 2
		cfg.Jitter = JitterNone
		cfg.InitialBackoff = time.Second
		cfg.MaxBackoff = time.Second
		cfg.TotalTimeout = 500 * time.Millisecond
		start := time.Now()
		calls := 0
		got, err := Do(context.Background(), cfg, retryAll, func(context.Context) (int, error) { calls++; return 5, original })
		if got != 5 || err != original || calls != 1 || !time.Now().Equal(start) {
			t.Fatal(got, err, calls, time.Since(start))
		}
		for _, own := range []bool{false, true} {
			parent, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			cfg := DefaultConfig()
			if own {
				cfg.TotalTimeout = 2 * time.Second
			}
			linked, err := Do(parent, cfg, nil, func(ctx context.Context) (context.Context, error) {
				deadline, _ := ctx.Deadline()
				expected, _ := parent.Deadline()
				if deadline != expected {
					t.Fatal(deadline, expected)
				}
				return ctx, nil
			})
			if err != nil || parent.Err() != nil {
				t.Fatal(err)
			}
			if own {
				if linked.Err() != context.Canceled {
					t.Fatal("derived context not canceled")
				}
			} else if linked != parent || linked.Err() != nil {
				t.Fatal("parent ownership changed")
			}
		}
		cfg = fastConfig()
		cfg.TotalTimeout = time.Second
		start = time.Now()
		value, err := Do(context.Background(), cfg, nil, func(ctx context.Context) (int, error) { <-ctx.Done(); return 8, nil })
		if value != 8 || err != nil || time.Since(start) != time.Second {
			t.Fatal(value, err)
		}
		cfg = fastConfig()
		cfg.MaxRetries = 2
		cfg.TotalTimeout = time.Second
		calls = 0
		_, err = Do(context.Background(), cfg, func(error) Decision { time.Sleep(2 * time.Second); return DecisionRetry }, func(context.Context) (int, error) { calls++; return 0, original })
		if calls != 1 || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, original) {
			t.Fatal(calls, err)
		}
	})
}

func TestWaitCancellationAndExactBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MaxRetries = 4
		cfg.Jitter = JitterNone
		cfg.InitialBackoff = time.Second
		cfg.MaxBackoff = 3 * time.Second
		start := time.Now()
		var times []time.Duration
		original := errors.New("fail")
		_, err := Do(context.Background(), cfg, retryAll, func(context.Context) (int, error) { times = append(times, time.Since(start)); return 2, original })
		if err != original || !reflect.DeepEqual(times, []time.Duration{0, time.Second, 3 * time.Second, 6 * time.Second, 9 * time.Second}) {
			t.Fatal(times, err)
		}
		ctx, cancel := context.WithCancelCause(context.Background())
		cause := errors.New("wait canceled")
		entered := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			got, err := Do(ctx, cfg, retryAll, func(context.Context) (int, error) { close(entered); return 9, original })
			if got != 9 || !errors.Is(err, original) || !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
				t.Error(got, err)
			}
		}()
		<-entered
		synctest.Wait()
		cancel(cause)
		<-done
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
		defer cancel2()
		cfg.InitialBackoff = time.Second
		cfg.MaxBackoff = time.Second
		calls := 0
		_, err = Do(ctx2, cfg, retryAll, func(context.Context) (int, error) { calls++; return 1, original })
		if calls != 1 || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(calls, err)
		}
	})
}

func TestNonCooperativeAndPanics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		entered := make(chan struct{})
		done := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			defer close(done)
			got, err := Do(ctx, DefaultConfig(), nil, func(context.Context) (int, error) { close(entered); <-gate; return 4, nil })
			if got != 4 || err != nil {
				t.Error(got, err)
			}
		}()
		<-entered
		cancel()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("abandoned callback")
		default:
		}
		close(gate)
		<-done
	})
	for _, where := range []string{"operation", "classifier"} {
		t.Run(where, func(t *testing.T) {
			marker := &domainError{33}
			calls := 0
			defer func() {
				if recover() != marker || calls != 1 {
					t.Fatal("panic changed", calls)
				}
			}()
			cfg := fastConfig()
			cfg.MaxRetries = 3
			Do(context.Background(), cfg, func(error) Decision { panic(marker) }, func(context.Context) (int, error) {
				calls++
				if where == "operation" {
					panic(marker)
				}
				return 5, errors.New("fail")
			})
		})
	}
}

func TestArithmeticExtremes(t *testing.T) {
	cfg := fastConfig()
	cfg.MaxBackoff = math.MaxInt64
	for _, mult := range []float64{1, 1.5, 2, math.MaxFloat64} {
		cfg.Multiplier = mult
		for _, k := range []int{0, 1, 2, 63, math.MaxInt} {
			got := backoff(cfg, k)
			if got < cfg.InitialBackoff || got > cfg.MaxBackoff {
				t.Fatal(mult, k, got)
			}
		}
	}
	cfg.Multiplier = 1
	cfg.InitialBackoff = math.MaxInt64
	if got := backoff(cfg, math.MaxInt); got != math.MaxInt64 {
		t.Fatal(got)
	}
	for _, cap := range []time.Duration{1, 100, math.MaxInt64} {
		if got := fullJitter(cap, func(n uint64) uint64 {
			if n != uint64(cap)+1 {
				t.Fatal(n)
			}
			return 0
		}); got != 0 {
			t.Fatal(got)
		}
		if got := fullJitter(cap, func(n uint64) uint64 { return n - 1 }); got != cap {
			t.Fatal(got)
		}
	}
	synctest.Test(t, func(t *testing.T) {
		cfg := fastConfig()
		cfg.MaxRetries = math.MaxInt
		calls := 0
		ctx, cancel := context.WithCancel(context.Background())
		_, err := Do(ctx, cfg, retryAll, func(context.Context) (int, error) {
			calls++
			if calls == 3 {
				cancel()
			}
			return 0, errors.New("fail")
		})
		if calls != 3 || !errors.Is(err, context.Canceled) {
			t.Fatal(calls, err)
		}
	})
}

func TestParentDeadlineCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cause := errors.New("parent deadline cause")
		ctx, cancel := context.WithTimeoutCause(context.Background(), time.Second, cause)
		defer cancel()
		cfg := fastConfig()
		cfg.MaxRetries = 2
		cfg.TotalTimeout = 2 * time.Second
		original := &domainError{99}
		calls := 0
		got, err := Do(ctx, cfg, retryAll, func(ctx context.Context) (int, error) { calls++; <-ctx.Done(); return 17, original })
		if calls != 1 || got != 17 || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, cause) || !errors.Is(err, original) {
			t.Fatal(got, err, calls)
		}
		got, err = Do(ctx, cfg, retryAll, func(context.Context) (int, error) { t.Fatal("expired parent called"); return 0, nil })
		if got != 0 || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, cause) || errors.Is(err, ErrInvalidCall) {
			t.Fatal(got, err)
		}
	})
}
