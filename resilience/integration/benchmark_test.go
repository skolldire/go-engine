package integration

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/skolldire/go-engine/resilience"

	"github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/concurrency"
	"github.com/skolldire/go-engine/ratelimit"
	"github.com/skolldire/go-engine/retry"
)

var benchmarkValue int
var benchmarkError error

func BenchmarkDirect(b *testing.B) {
	op := func(context.Context) (int, error) { return 42, nil }
	b.ReportAllocs()
	for b.Loop() {
		benchmarkValue, benchmarkError = op(context.Background())
	}
}
func benchDependencies(b *testing.B, mask int) (resilience.Config, resilience.Dependencies) {
	b.Helper()
	cfg := resilience.DefaultConfig()
	deps := resilience.Dependencies{}
	var err error
	if mask&1 != 0 {
		cfg.Weight = 1
		deps.Limiter, err = concurrency.NewLimiter(concurrency.LimiterConfig{Capacity: 16})
		if err != nil {
			b.Fatal(err)
		}
	}
	if mask&2 != 0 {
		cfg.Tokens = 1
		deps.RateLimiter, err = ratelimit.New(ratelimit.Config{RatePerSecond: math.SmallestNonzeroFloat64, Burst: math.MaxInt})
		if err != nil {
			b.Fatal(err)
		}
	}
	if mask&4 != 0 {
		c := circuitbreaker.DefaultConfig()
		c.Name = "bench"
		c.Interval = time.Hour
		deps.Breaker, err = circuitbreaker.New(c, circuitbreaker.Dependencies{Classify: func(error) bool { return true }})
		if err != nil {
			b.Fatal(err)
		}
	}
	return cfg, deps
}
func BenchmarkWrapExecution(b *testing.B) {
	for _, tc := range []struct {
		name string
		mask int
	}{{"none", 0}, {"concurrency", 1}, {"ratelimit", 2}, {"breaker", 4}, {"all", 7}} {
		b.Run(tc.name, func(b *testing.B) {
			cfg, deps := benchDependencies(b, tc.mask)
			fn, err := resilience.Wrap(func(context.Context) (int, error) { return 42, nil }, cfg, deps)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkValue, benchmarkError = fn(context.Background())
			}
		})
	}
}
func BenchmarkConstruction(b *testing.B) {
	cfg, deps := benchDependencies(b, 7)
	op := func(context.Context) (int, error) { return 42, nil }
	b.ReportAllocs()
	for b.Loop() {
		fn, err := resilience.Wrap(op, cfg, deps)
		if err != nil || fn == nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkBudgets(b *testing.B) {
	for _, tc := range []struct {
		name                      string
		total, attempt, admission time.Duration
	}{{"total", time.Hour, 0, 0}, {"attempt", 0, time.Hour, 0}, {"admission", 0, 0, time.Hour}} {
		b.Run(tc.name, func(b *testing.B) {
			cfg, deps := benchDependencies(b, 1)
			cfg.TotalTimeout = tc.total
			cfg.AttemptTimeout = tc.attempt
			cfg.AdmissionTimeout = tc.admission
			fn, err := resilience.Wrap(func(context.Context) (int, error) { return 42, nil }, cfg, deps)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkValue, benchmarkError = fn(context.Background())
			}
		})
	}
}
func BenchmarkRejections(b *testing.B) {
	for _, stage := range []string{"ratelimit", "concurrency", "breaker"} {
		b.Run(stage, func(b *testing.B) {
			cfg, deps := benchDependencies(b, 7)
			switch stage {
			case "ratelimit":
				cfg.Tokens = math.MaxInt
				cfg.Tokens--
				deps.RateLimiter, _ = ratelimit.New(ratelimit.Config{RatePerSecond: 1, Burst: 1})
			case "concurrency":
				cfg.Weight = 17
			case "breaker":
				deps.Breaker, _ = circuitbreaker.New(func() circuitbreaker.Config {
					c := circuitbreaker.DefaultConfig()
					c.Name = "open"
					c.RequestThreshold = 1
					c.OpenTimeout = time.Hour
					return c
				}(), circuitbreaker.Dependencies{Classify: func(error) bool { return true }})
				circuitbreaker.Execute(context.Background(), deps.Breaker.(*circuitbreaker.Breaker), func(context.Context) (int, error) { return 0, errors.New("remote") })
			}
			fn, err := resilience.Wrap(func(context.Context) (int, error) { return 42, nil }, cfg, deps)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkValue, benchmarkError = fn(context.Background())
			}
		})
	}
}
func BenchmarkRetry(b *testing.B) {
	cfg := config()
	cfg.Retry.MaxRetries = 2
	original := errors.New("transient")
	calls := 0
	fn, err := resilience.Wrap(func(context.Context) (int, error) {
		calls++
		if calls < 3 {
			return 0, original
		}
		return 42, nil
	}, cfg, resilience.Dependencies{Classify: func(error) retry.Decision { return retry.DecisionRetry }})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		calls = 0
		benchmarkValue, benchmarkError = fn(context.Background())
	}
}
func BenchmarkConcurrent(b *testing.B) {
	cfg, deps := benchDependencies(b, 7)
	fn, err := resilience.Wrap(func(context.Context) (int, error) { return 42, nil }, cfg, deps)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			value, err := fn(context.Background())
			if value != 42 || err != nil {
				b.Fatal(value, err)
			}
		}
	})
}

func BenchmarkBoundarySuccess(b *testing.B) {
	cfg := resilience.DefaultConfig()
	cfg.Retry.MaxRetries = 3
	c := circuitbreaker.DefaultConfig()
	c.Name = "bench"
	breaker, err := circuitbreaker.New(c, circuitbreaker.Dependencies{Classify: func(error) bool { return true }})
	if err != nil {
		b.Fatal(err)
	}
	fn, err := resilience.Wrap(func(context.Context) (int, error) { return 42, nil }, cfg, resilience.Dependencies{Breaker: breaker, Classify: retryAll})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		benchmarkValue, benchmarkError = fn(context.Background())
	}
}
