package circuitbreaker

import (
	"context"
	"errors"
	"math"
	"math/big"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type remoteError struct{ code int }

func (e *remoteError) Error() string { return "remote failure" }
func testConfig() Config             { cfg := DefaultConfig(); cfg.Name = "test"; return cfg }
func newTestBreaker(t *testing.T, cfg Config, classify func(error) bool) *Breaker {
	t.Helper()
	b, err := New(cfg, Dependencies{classify})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func remote(error) bool { return true }
func checkCounts(t *testing.T, c Counts) {
	t.Helper()
	sum := new(big.Int).SetUint64(c.InFlight)
	for _, v := range []uint64{c.Successes, c.Failures, c.Ignored, c.Panics} {
		sum.Add(sum, new(big.Int).SetUint64(v))
	}
	if !sum.IsUint64() || sum.Uint64() != c.Requests {
		t.Fatal("invalid counters", c)
	}
}
func trip(t *testing.T, b *Breaker) {
	t.Helper()
	for range b.cfg.RequestThreshold {
		_, err := Execute(context.Background(), b, func(context.Context) (int, error) { return 1, &remoteError{1} })
		if err == nil {
			t.Fatal("missing failure")
		}
	}
	if b.State() != StateOpen {
		t.Fatal(b.Stats())
	}
}

func TestConfigAndValidation(t *testing.T) {
	cfg := DefaultConfig()
	if cfg != (Config{RequestThreshold: 5, FailureRate: 0.5, Interval: time.Minute, OpenTimeout: 30 * time.Second, HalfOpenMax: 1}) {
		t.Fatal(cfg)
	}
	cases := []struct {
		field string
		edit  func(*Config)
	}{
		{"Name", func(c *Config) { c.Name = "" }},
		{"RequestThreshold", func(c *Config) { c.RequestThreshold = 0 }},
		{"FailureRate", func(c *Config) { c.FailureRate = 0 }},
		{"FailureRate", func(c *Config) { c.FailureRate = 1.1 }},
		{"FailureRate", func(c *Config) { c.FailureRate = math.NaN() }},
		{"FailureRate", func(c *Config) { c.FailureRate = math.Inf(1) }},
		{"FailureRate", func(c *Config) { c.FailureRate = math.Inf(-1) }},
		{"Interval", func(c *Config) { c.Interval = 0 }},
		{"OpenTimeout", func(c *Config) { c.OpenTimeout = -1 }},
		{"HalfOpenMax", func(c *Config) { c.HalfOpenMax = 0 }},
	}
	for _, tc := range cases {
		c := testConfig()
		tc.edit(&c)
		_, err := New(c, Dependencies{})
		var ce *ConfigError
		if !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tc.field {
			t.Fatal(err)
		}
	}
	_, err := New(testConfig(), Dependencies{})
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "Classify" || !errors.Is(err, ErrInvalidConfig) || errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
	for _, name := range []string{" ", "\xff\x00", strings.Repeat("x", 4096)} {
		cfg := testConfig()
		cfg.Name = name
		cfg.RequestThreshold = math.MaxUint64
		cfg.HalfOpenMax = math.MaxUint64
		cfg.Interval = math.MaxInt64
		cfg.OpenTimeout = math.MaxInt64
		cfg.FailureRate = math.SmallestNonzeroFloat64
		if err := cfg.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	b := newTestBreaker(t, testConfig(), remote)
	casesCall := []struct {
		ctx   context.Context
		b     *Breaker
		op    func(context.Context) (int, error)
		field string
	}{
		{nil, nil, nil, "ctx"}, {context.Background(), nil, nil, "breaker"}, {context.Background(), b, nil, "operation"},
	}
	for _, tc := range casesCall {
		got, err := Execute(tc.ctx, tc.b, tc.op)
		if got != 0 || !errors.Is(err, ErrInvalidCall) || !strings.Contains(err.Error(), tc.field) {
			t.Fatal(got, err)
		}
	}
	if b.Stats() != (Counts{State: StateClosed}) {
		t.Fatal(b.Stats())
	}
}

func TestOutcomesContextAndClassification(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := &remoteError{7}
		business := errors.New("business")
		classifications := 0
		cfg := testConfig()
		before := cfg
		var b *Breaker
		b = newTestBreaker(t, cfg, func(err error) bool { classifications++; _ = b.State(); _ = b.Stats(); return errors.Is(err, original) })
		cfg.Name = "mutated"
		if b.cfg != before {
			t.Fatal("configuration retained")
		}
		type key struct{}
		parent := context.WithValue(context.Background(), key{}, "v")
		ctx, cancel := context.WithTimeout(parent, time.Hour)
		defer cancel()
		result := map[string]int{"value": 42}
		value, err := Execute(ctx, b, func(got context.Context) (map[string]int, error) {
			if got != ctx || got.Value(key{}) != "v" {
				t.Fatal("context replaced")
			}
			return result, original
		})
		if err != original || value["value"] != 42 {
			t.Fatal(value, err)
		}
		value["value"] = 10
		if result["value"] != 10 {
			t.Fatal("result copied")
		}
		var typed *remoteError
		if !errors.As(err, &typed) || typed != original {
			t.Fatal(err)
		}
		_, err = Execute(ctx, b, func(context.Context) (int, error) { return 3, business })
		if err != business {
			t.Fatal(err)
		}
		_, err = Execute(ctx, b, func(context.Context) (int, error) { return 5, nil })
		if err != nil || classifications != 2 {
			t.Fatal(err, classifications)
		}
		got := b.Stats()
		if got != (Counts{State: StateClosed, Requests: 3, Successes: 1, Failures: 1, Ignored: 1}) {
			t.Fatal(got)
		}
		got.Requests = 999
		if b.Stats().Requests != 3 {
			t.Fatal("mutable stats")
		}
		cause := errors.New("caller canceled")
		canceled, stop := context.WithCancelCause(ctx)
		stop(cause)
		v, err := Execute(canceled, b, func(context.Context) (int, error) { t.Fatal("pre-canceled operation"); return 1, nil })
		if v != 0 || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || errors.Is(err, ErrInvalidCall) || b.Stats().Requests != 3 {
			t.Fatal(v, err)
		}
		active, done := context.WithCancel(ctx)
		v, err = Execute(active, b, func(received context.Context) (int, error) { done(); return 9, nil })
		if v != 9 || err != nil {
			t.Fatal(v, err)
		}
		_, err = Execute(ctx, b, func(context.Context) (int, error) { return 0, context.Canceled })
		if err != context.Canceled || classifications != 3 || b.Stats().Ignored != 2 {
			t.Fatal(err, b.Stats())
		}
		expired, expire := context.WithTimeoutCause(ctx, time.Second, cause)
		defer expire()
		time.Sleep(time.Second)
		_, err = Execute(expired, b, func(context.Context) (int, error) { t.Fatal("expired callback"); return 0, nil })
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	})
}

func TestThresholdAndDenominator(t *testing.T) {
	for _, tc := range []struct {
		successes, failures int
		open                bool
	}{{3, 2, false}, {2, 3, true}, {0, 4, false}, {0, 5, true}} {
		b := newTestBreaker(t, testConfig(), remote)
		for range tc.successes {
			Execute(context.Background(), b, func(context.Context) (int, error) { return 1, nil })
		}
		for range tc.failures {
			Execute(context.Background(), b, func(context.Context) (int, error) { return 2, &remoteError{1} })
		}
		if (b.State() == StateOpen) != tc.open {
			t.Fatal(tc, b.Stats())
		}
		checkCounts(t, b.Stats())
	}
	synctest.Test(t, func(t *testing.T) {
		original := &remoteError{3}
		business := errors.New("business")
		b := newTestBreaker(t, testConfig(), func(e error) bool { return errors.Is(e, original) })
		for range 50 {
			Execute(context.Background(), b, func(context.Context) (int, error) { return 0, business })
		}
		gate := make(chan struct{})
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() { Execute(context.Background(), b, func(context.Context) (int, error) { <-gate; return 9, nil }) })
		}
		synctest.Wait()
		if b.Stats().InFlight != 8 {
			t.Fatal(b.Stats())
		}
		for range b.cfg.RequestThreshold {
			Execute(context.Background(), b, func(context.Context) (int, error) { return 1, original })
		}
		close(gate)
		wg.Wait()
		if b.State() != StateOpen || b.Stats().Requests != 0 {
			t.Fatal(b.Stats())
		}
	})
}

func TestTransitionsAndWindows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.HalfOpenMax = 2
		b := newTestBreaker(t, cfg, remote)
		Execute(context.Background(), b, func(context.Context) (int, error) { return 1, nil })
		time.Sleep(cfg.Interval - time.Nanosecond)
		if b.Stats().Requests != 1 {
			t.Fatal(b.Stats())
		}
		time.Sleep(time.Nanosecond)
		if b.Stats() != (Counts{State: StateClosed}) {
			t.Fatal(b.Stats())
		}
		trip(t, b)
		called := false
		value, err := Execute(context.Background(), b, func(context.Context) (int, error) { called = true; return 42, nil })
		if value != 0 || !errors.Is(err, ErrOpen) || called || b.Stats().Requests != 0 {
			t.Fatal(value, err)
		}
		time.Sleep(cfg.OpenTimeout - time.Nanosecond)
		if b.State() != StateOpen {
			t.Fatal(b.State())
		}
		time.Sleep(time.Nanosecond)
		if b.State() != StateHalfOpen {
			t.Fatal(b.State())
		}
		Execute(context.Background(), b, func(context.Context) (int, error) { return 1, nil })
		if b.State() != StateHalfOpen || b.Stats().Successes != 1 {
			t.Fatal(b.Stats())
		}
		Execute(context.Background(), b, func(context.Context) (int, error) { return 1, nil })
		if b.Stats() != (Counts{State: StateClosed}) {
			t.Fatal(b.Stats())
		}
		trip(t, b)
		time.Sleep(cfg.OpenTimeout)
		_, err = Execute(context.Background(), b, func(context.Context) (int, error) { return 8, &remoteError{2} })
		if err == nil || b.State() != StateOpen {
			t.Fatal(err, b.Stats())
		}
		time.Sleep(cfg.OpenTimeout - time.Nanosecond)
		if b.State() != StateOpen {
			t.Fatal(b.State())
		}
		time.Sleep(time.Nanosecond)
		if b.Stats().State != StateHalfOpen {
			t.Fatal(b.Stats())
		}
	})
}

func TestConcurrentProbeLimitsAndLateResults(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.HalfOpenMax = 2
		var classified atomic.Int64
		b := newTestBreaker(t, cfg, func(error) bool { classified.Add(1); return true })
		trip(t, b)
		time.Sleep(cfg.OpenTimeout)
		gate := make(chan struct{})
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() { Execute(context.Background(), b, func(context.Context) (int, error) { <-gate; return 4, nil }) })
		}
		synctest.Wait()
		if b.Stats().InFlight != 2 {
			t.Fatal(b.Stats())
		}
		before := classified.Load()
		for range 100 {
			_, err := Execute(context.Background(), b, func(context.Context) (int, error) { t.Error("rejected probe executed"); return 0, nil })
			if err != ErrProbeLimit {
				t.Fatal(err)
			}
		}
		if classified.Load() != before {
			t.Fatal("rejections classified")
		}
		close(gate)
		wg.Wait()
		if b.State() != StateClosed {
			t.Fatal(b.Stats())
		}
		gate = make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			value, err := Execute(context.Background(), b, func(context.Context) (int, error) { <-gate; return 19, &remoteError{6} })
			if value != 19 || err == nil {
				t.Error(value, err)
			}
		}()
		synctest.Wait()
		old := b.current
		time.Sleep(cfg.Interval)
		if b.Stats().Requests != 0 || b.current == old {
			t.Fatal(b.Stats())
		}
		close(gate)
		<-done
		if b.Stats().Requests != 0 || classified.Load() != before+1 {
			t.Fatal(b.Stats(), classified.Load())
		}
	})
}

func catchPanic(f func()) (value any) { defer func() { value = recover() }(); f(); return nil }
func TestAbnormalTerminationAndProbeRelease(t *testing.T) {
	for _, where := range []string{"operation", "classifier"} {
		for _, goexit := range []bool{false, true} {
			synctest.Test(t, func(t *testing.T) {
				marker := &remoteError{71}
				cfg := testConfig()
				cfg.RequestThreshold = 1
				b := newTestBreaker(t, cfg, remote)
				trip(t, b)
				time.Sleep(cfg.OpenTimeout)
				b.classify = func(error) bool {
					if where == "classifier" {
						if goexit {
							runtime.Goexit()
						}
						panic(marker)
					}
					return true
				}
				done := make(chan struct{})
				go func() {
					defer close(done)
					work := func() {
						Execute(context.Background(), b, func(context.Context) (int, error) {
							if where == "operation" {
								if goexit {
									runtime.Goexit()
								}
								panic(marker)
							}
							return 5, errors.New("failure")
						})
					}
					if goexit {
						work()
						t.Error("Goexit returned")
					} else if catchPanic(work) != marker {
						t.Error("panic changed")
					}
				}()
				<-done
				if b.Stats() != (Counts{State: StateHalfOpen, Requests: 1, Panics: 1}) {
					t.Fatal(where, goexit, b.Stats())
				}
				_, err := Execute(context.Background(), b, func(context.Context) (int, error) { return 9, nil })
				if err != nil || b.State() != StateClosed {
					t.Fatal(err, b.Stats())
				}
			})
		}
	}
}

func TestIgnoredRecoveryAndNonCooperative(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		remoteErr := errors.New("remote")
		b := newTestBreaker(t, testConfig(), func(err error) bool { return errors.Is(err, remoteErr) })
		for range b.cfg.RequestThreshold {
			Execute(context.Background(), b, func(context.Context) (int, error) { return 0, remoteErr })
		}
		time.Sleep(b.cfg.OpenTimeout)
		for range 20 {
			Execute(context.Background(), b, func(context.Context) (int, error) { return 1, errors.New("business") })
		}
		time.Sleep(24 * time.Hour)
		if b.Stats() != (Counts{State: StateHalfOpen, Requests: 20, Ignored: 20}) {
			t.Fatal(b.Stats())
		}
		gate := make(chan struct{})
		done := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			defer close(done)
			got, err := Execute(ctx, b, func(context.Context) (int, error) { <-gate; return 13, nil })
			if got != 13 || err != nil {
				t.Error(got, err)
			}
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		if b.Stats().InFlight != 1 {
			t.Fatal(b.Stats())
		}
		select {
		case <-done:
			t.Fatal("abandoned callback")
		default:
		}
		close(gate)
		<-done
		if b.State() != StateClosed {
			t.Fatal(b.Stats())
		}
	})
	a := newTestBreaker(t, testConfig(), remote)
	b := newTestBreaker(t, testConfig(), remote)
	trip(t, a)
	if b.State() != StateClosed {
		t.Fatal("shared name shared state")
	}
}

func TestCounterRollover(t *testing.T) {
	for _, state := range []State{StateClosed, StateHalfOpen} {
		cfg := testConfig()
		cfg.HalfOpenMax = 10
		b := newTestBreaker(t, cfg, remote)
		b.start(state, time.Now())
		old := b.current
		old.counts.Requests = math.MaxUint64
		old.counts.Successes = math.MaxUint64 - 1
		old.counts.InFlight = 1
		g, err := b.admit(context.Background())
		if err != nil || g == old || g.counts != (Counts{State: state, Requests: 1, InFlight: 1}) {
			t.Fatal(g, err)
		}
		b.finish(old, success)
		if g.counts.Requests != 1 || g.counts.InFlight != 1 {
			t.Fatal(g.counts)
		}
		b.finish(g, success)
		checkCounts(t, b.Stats())
		for _, result := range []outcome{success, failure, ignored, abnormal} {
			cfg.RequestThreshold = math.MaxUint64
			b = newTestBreaker(t, cfg, remote)
			b.start(state, time.Now())
			old = b.current
			old.counts.InFlight = 1
			*terminalCount(&old.counts, result) = math.MaxUint64
			b.finish(old, result)
			if b.current == old {
				t.Fatal("did not rotate")
			}
			c := b.Stats()
			checkCounts(t, c)
			if state == StateHalfOpen && result == failure {
				if c.State != StateOpen {
					t.Fatal(c)
				}
			} else if c.Requests != 1 || *terminalCount(&c, result) != 1 || c.InFlight != 0 {
				t.Fatal(c)
			}
		}
	}
	b := newTestBreaker(t, testConfig(), remote)
	b.start(StateOpen, time.Now())
	expires := b.current.expires
	old := b.current
	b.rotate(time.Now().Add(time.Second))
	if b.current == old || b.current.expires != expires || b.State() != StateOpen {
		t.Fatal(b.Stats())
	}
}

func TestExactRateArithmetic(t *testing.T) {
	for _, completed := range []uint64{1, 2, 5, 100, 1 << 53, math.MaxUint64} {
		for _, failures := range []uint64{0, 1, completed / 2, completed - 1, completed} {
			for _, rate := range []float64{math.SmallestNonzeroFloat64, 0.1, 0.5, math.Nextafter(1, 0), 1} {
				got := reachedRate(failures, completed, rate)
				ratio := new(big.Rat).SetFrac(new(big.Int).SetUint64(failures), new(big.Int).SetUint64(completed))
				want := ratio.Cmp(new(big.Rat).SetFloat64(rate)) >= 0
				if got != want {
					t.Fatal(completed, failures, rate, got, want)
				}
			}
		}
	}
}

func TestLateAbnormalAndAccumulatedRecovery(t *testing.T) {
	for _, goexit := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			b := newTestBreaker(t, cfg, remote)
			gate := make(chan struct{})
			done := make(chan struct{})
			marker := &remoteError{8}
			go func() {
				defer close(done)
				work := func() {
					Execute(context.Background(), b, func(context.Context) (int, error) {
						<-gate
						if goexit {
							runtime.Goexit()
						}
						panic(marker)
					})
				}
				if goexit {
					work()
				} else if catchPanic(work) != marker {
					t.Error("late panic changed")
				}
			}()
			synctest.Wait()
			time.Sleep(cfg.Interval)
			if b.Stats().Requests != 0 {
				t.Fatal(b.Stats())
			}
			close(gate)
			<-done
			if b.Stats() != (Counts{State: StateClosed}) {
				t.Fatal("late abnormal exit changed generation", b.Stats())
			}
		})
	}
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.HalfOpenMax = 2
		classified := errors.New("remote")
		business := errors.New("business")
		b := newTestBreaker(t, cfg, func(err error) bool { return errors.Is(err, classified) })
		for range cfg.RequestThreshold {
			Execute(context.Background(), b, func(context.Context) (int, error) { return 0, classified })
		}
		time.Sleep(cfg.OpenTimeout)
		Execute(context.Background(), b, func(context.Context) (int, error) { return 1, nil })
		Execute(context.Background(), b, func(context.Context) (int, error) { return 2, business })
		catchPanic(func() { Execute(context.Background(), b, func(context.Context) (int, error) { panic(business) }) })
		if b.Stats() != (Counts{State: StateHalfOpen, Requests: 3, Successes: 1, Ignored: 1, Panics: 1}) {
			t.Fatal(b.Stats())
		}
		Execute(context.Background(), b, func(context.Context) (int, error) { return 3, nil })
		if b.State() != StateClosed {
			t.Fatal(b.Stats())
		}
	})
}
