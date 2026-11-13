package resilience

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/retry"
)

type foreignRate struct {
	wait func(context.Context, int) error
}

func (r *foreignRate) Wait(ctx context.Context, n int) error { return r.wait(ctx, n) }

type foreignLimiter struct {
	acquire func(context.Context, int64) (func(), error)
}

func (l *foreignLimiter) Acquire(ctx context.Context, n int64) (func(), error) {
	return l.acquire(ctx, n)
}

type foreignBreaker struct {
	run func(context.Context, func(context.Context) error) error
}

func (b *foreignBreaker) Run(ctx context.Context, fn func(context.Context) error) error {
	return b.run(ctx, fn)
}
func localConfig() Config {
	cfg := DefaultConfig()
	cfg.Retry.InitialBackoff = time.Nanosecond
	cfg.Retry.MaxBackoff = time.Nanosecond
	cfg.Retry.Jitter = retry.JitterNone
	return cfg
}
func mustWrap[T any](t testing.TB, op client.Operation[T], cfg Config, deps Dependencies) client.Operation[T] {
	t.Helper()
	f, err := Wrap(op, cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

type operationError struct{ cause error }

func (*operationError) Error() string   { return "domain" }
func (e *operationError) Unwrap() error { return e.cause }
func TestIndependentInterfacesAndOrder(t *testing.T) {
	var order []string
	cfg := localConfig()
	cfg.Tokens = 3
	cfg.Weight = 2
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "parent")
	cause := errors.New("original")
	failure := &operationError{cause}
	payload := map[string]int{"value": 7}
	deps := Dependencies{
		RateLimiter: &foreignRate{func(ctx context.Context, n int) error {
			if ctx != parent || n != 3 {
				t.Fatal("rate args")
			}
			order = append(order, "rate")
			return nil
		}},
		Limiter: &foreignLimiter{func(ctx context.Context, n int64) (func(), error) {
			if ctx != parent || n != 2 {
				t.Fatal("limiter args")
			}
			order = append(order, "acquire")
			return func() { order = append(order, "release") }, nil
		}},
		Breaker: &foreignBreaker{func(ctx context.Context, fn func(context.Context) error) error {
			order = append(order, "run")
			return fn(context.WithValue(ctx, key{}, "derived"))
		}},
		Classify: func(err error) retry.Decision {
			if err != failure {
				t.Fatal("error changed")
			}
			order = append(order, "classify")
			return retry.DecisionStop
		},
	}
	f := mustWrap(t, func(ctx context.Context) (map[string]int, error) {
		if ctx.Value(key{}) != "derived" {
			t.Fatal("breaker context replaced")
		}
		order = append(order, "io")
		return payload, failure
	}, cfg, deps)
	if len(order) != 0 {
		t.Fatal("Wrap executed dependencies")
	}
	cfg.Tokens = 0
	cfg.Weight = 0
	value, err := f(parent)
	var typed *operationError
	if err != failure || !errors.Is(err, cause) || !errors.As(err, &typed) || typed != failure {
		t.Fatal("original error not preserved")
	}
	value["value"] = 9
	if payload["value"] != 9 {
		t.Fatal("result copied")
	}
	if !reflect.DeepEqual(order, []string{"rate", "acquire", "run", "io", "release", "classify"}) {
		t.Fatal(order)
	}
}
func TestForeignRejectionStopsWithoutClassifier(t *testing.T) {
	original := &operationError{errors.New("backend rejection")}
	cfg := localConfig()
	cfg.Retry.MaxRetries = 4
	calls := 0
	f := mustWrap(t, func(context.Context) (int, error) { t.Fatal("rejected operation entered"); return 0, nil }, cfg, Dependencies{Breaker: &foreignBreaker{func(context.Context, func(context.Context) error) error { calls++; return original }}, Classify: func(error) retry.Decision { t.Fatal("local rejection reached classifier"); return retry.DecisionRetry }})
	value, err := f(context.Background())
	var typed *operationError
	if value != 0 || calls != 1 || !errors.Is(err, ErrRejected) || !errors.Is(err, original) || !errors.As(err, &typed) || typed != original {
		t.Fatal("rejection cause lost", err)
	}
}
func TestEnteredErrorIsNotRejection(t *testing.T) {
	failure := &operationError{}
	classified := 0
	cfg := localConfig()
	cfg.Retry.MaxRetries = 1
	f := mustWrap(t, func(context.Context) (int, error) { return 42, failure }, cfg, Dependencies{Breaker: &foreignBreaker{func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }}, Classify: func(err error) retry.Decision {
		classified++
		if err != failure {
			t.Fatal("identity changed")
		}
		return retry.DecisionStop
	}})
	value, err := f(context.Background())
	if value != 42 || err != failure || errors.Is(err, ErrRejected) || classified != 1 {
		t.Fatal(value, err, classified)
	}
}
func TestTypedNilInterfacesAndValidation(t *testing.T) {
	var b *foreignBreaker
	var l *foreignLimiter
	var r *foreignRate
	for _, tc := range []struct {
		field string
		deps  Dependencies
	}{{"Breaker", Dependencies{Breaker: b}}, {"Limiter", Dependencies{Limiter: l}}, {"RateLimiter", Dependencies{RateLimiter: r}}} {
		_, err := Wrap(func(context.Context) (int, error) { return 1, nil }, DefaultConfig(), tc.deps)
		var ce *ConfigError
		if !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tc.field {
			t.Fatal(tc.field, err)
		}
	}
	pass := &foreignBreaker{func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }}
	for _, tc := range []struct {
		field string
		cfg   Config
		deps  Dependencies
	}{
		{"Order", func() Config { c := DefaultConfig(); c.Order = ""; return c }(), Dependencies{Breaker: pass}},
		{"Classify", func() Config { c := DefaultConfig(); c.Retry.MaxRetries = 1; return c }(), Dependencies{}},
		{"Weight", DefaultConfig(), Dependencies{Limiter: &foreignLimiter{}}},
		{"Tokens", DefaultConfig(), Dependencies{RateLimiter: &foreignRate{}}},
	} {
		_, err := Wrap(func(context.Context) (int, error) { return 1, nil }, tc.cfg, tc.deps)
		var ce *ConfigError
		if !errors.As(err, &ce) || ce.Field != tc.field {
			t.Fatal(tc.field, err)
		}
	}
}
func TestNilReleaseAndSingleRelease(t *testing.T) {
	for _, mode := range []string{"nil", "success", "failure", "panic", "exit", "reject"} {
		t.Run(mode, func(t *testing.T) {
			var releases atomic.Int64
			cfg := localConfig()
			cfg.Weight = 1
			deps := Dependencies{Limiter: &foreignLimiter{func(context.Context, int64) (func(), error) {
				if mode == "nil" {
					return nil, nil
				}
				return func() { releases.Add(1) }, nil
			}}}
			if mode == "reject" {
				deps.Breaker = &foreignBreaker{func(context.Context, func(context.Context) error) error { return errors.New("rejected") }}
			}
			failure := errors.New("original")
			f := mustWrap(t, func(context.Context) (int, error) {
				switch mode {
				case "panic":
					panic("same panic")
				case "exit":
					runtime.Goexit()
				case "failure":
					return 7, failure
				}
				return 7, nil
			}, cfg, deps)
			done := make(chan struct{})
			var panicValue any
			go func() {
				defer close(done)
				defer func() { panicValue = recover() }()
				_, err := f(context.Background())
				if mode == "failure" && err != failure {
					t.Error("changed error")
				}
			}()
			<-done
			if mode == "panic" && panicValue != "same panic" {
				t.Fatal("panic changed")
			}
			expected := int64(1)
			if mode == "nil" {
				expected = 0
			}
			if releases.Load() != expected {
				t.Fatal(releases.Load())
			}
		})
	}
}
func TestMultipleCallbackViolationIsObservable(t *testing.T) {
	calls, releases := 0, 0
	failure := errors.New("second callback")
	cfg := localConfig()
	cfg.Weight = 1
	f := mustWrap(t, func(context.Context) (int, error) { calls++; return calls, failure }, cfg, Dependencies{Limiter: &foreignLimiter{func(context.Context, int64) (func(), error) { return func() { releases++ }, nil }}, Breaker: &foreignBreaker{func(ctx context.Context, fn func(context.Context) error) error { _ = fn(ctx); return fn(ctx) }}})
	value, err := f(context.Background())
	if value != 2 || calls != 2 || err != failure || releases != 1 {
		t.Fatal("contract violation was hidden", value, calls, releases)
	}
}
func TestLocalAdmissionStopsAndCauses(t *testing.T) {
	for _, stage := range []string{"rate", "limiter"} {
		failure := errors.New("admission")
		cfg := localConfig()
		cfg.Retry.MaxRetries = 2
		deps := Dependencies{Classify: func(error) retry.Decision { t.Fatal("admission reached classifier"); return retry.DecisionRetry }}
		if stage == "rate" {
			cfg.Tokens = 1
			deps.RateLimiter = &foreignRate{func(context.Context, int) error { return failure }}
		} else {
			cfg.Weight = 1
			deps.Limiter = &foreignLimiter{func(context.Context, int64) (func(), error) { return nil, failure }}
		}
		f := mustWrap(t, func(context.Context) (int, error) { t.Fatal("admission executed I/O"); return 0, nil }, cfg, deps)
		_, err := f(context.Background())
		if !errors.Is(err, ErrAdmission) || !errors.Is(err, failure) || !strings.Contains(err.Error(), map[string]string{"rate": "ratelimit", "limiter": "concurrency"}[stage]) {
			t.Fatal(err)
		}
	}
}
func TestBudgetsWithForeignImplementations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, stage := range []string{"io", "rate", "limiter"} {
			cfg := localConfig()
			cfg.TotalTimeout = 10 * time.Second
			cfg.AttemptTimeout = time.Second
			cfg.AdmissionTimeout = 2 * time.Second
			deps := Dependencies{}
			if stage == "rate" {
				cfg.Tokens = 1
				deps.RateLimiter = &foreignRate{func(ctx context.Context, _ int) error { <-ctx.Done(); return ctx.Err() }}
			}
			if stage == "limiter" {
				cfg.Weight = 1
				deps.Limiter = &foreignLimiter{func(ctx context.Context, _ int64) (func(), error) { <-ctx.Done(); return nil, ctx.Err() }}
			}
			calls := 0
			f := mustWrap(t, func(ctx context.Context) (int, error) { calls++; <-ctx.Done(); return 17, ctx.Err() }, cfg, deps)
			value, err := f(context.Background())
			marker := ErrAdmissionTimeout
			if stage == "io" {
				marker = ErrAttemptTimeout
			}
			if !errors.Is(err, marker) || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(stage, err)
			}
			if stage == "io" {
				if value != 17 || calls != 1 {
					t.Fatal(value, calls)
				}
			} else if value != 0 || calls != 0 || !errors.Is(err, ErrAdmission) {
				t.Fatal(value, calls)
			}
		}
	})
}
