package circuitbreaker

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

var benchmarkValue int
var benchmarkError error

func benchBreaker(b *testing.B) *Breaker {
	b.Helper()
	cfg := DefaultConfig()
	cfg.Name = "bench"
	cfg.RequestThreshold = math.MaxUint64
	cfg.Interval = time.Hour
	breaker, err := New(cfg, Dependencies{func(error) bool { return true }})
	if err != nil {
		b.Fatal(err)
	}
	return breaker
}
func BenchmarkDirect(b *testing.B) {
	op := func(context.Context) (int, error) { return 42, nil }
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		benchmarkValue, benchmarkError = op(ctx)
	}
}
func BenchmarkClosed(b *testing.B) {
	for _, name := range []string{"success", "failure", "ignored"} {
		b.Run(name, func(b *testing.B) {
			breaker := benchBreaker(b)
			original := errors.New("failure")
			if name == "ignored" {
				breaker.classify = func(error) bool { return false }
			}
			op := func(context.Context) (int, error) {
				if name == "success" {
					return 42, nil
				}
				return 42, original
			}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkValue, benchmarkError = Execute(context.Background(), breaker, op)
			}
		})
	}
}
func BenchmarkOpen(b *testing.B) {
	breaker := benchBreaker(b)
	breaker.cfg.OpenTimeout = time.Hour
	breaker.start(StateOpen, time.Now())
	op := func(context.Context) (int, error) { return 42, nil }
	b.ReportAllocs()
	for b.Loop() {
		benchmarkValue, benchmarkError = Execute(context.Background(), breaker, op)
	}
}
func BenchmarkHalfOpen(b *testing.B) {
	for _, reject := range []bool{false, true} {
		name := "admit"
		if reject {
			name = "reject"
		}
		b.Run(name, func(b *testing.B) {
			breaker := benchBreaker(b)
			breaker.start(StateHalfOpen, time.Now())
			breaker.cfg.HalfOpenMax = math.MaxUint64
			if reject {
				breaker.current.counts.InFlight = math.MaxUint64
				breaker.current.counts.Requests = math.MaxUint64
			}
			breaker.classify = func(error) bool { return false }
			original := errors.New("business")
			op := func(context.Context) (int, error) { return 42, original }
			b.ReportAllocs()
			for b.Loop() {
				benchmarkValue, benchmarkError = Execute(context.Background(), breaker, op)
			}
		})
	}
}
func BenchmarkWindowRenewal(b *testing.B) {
	breaker := benchBreaker(b)
	now := time.Now()
	b.ReportAllocs()
	for b.Loop() {
		now = now.Add(breaker.cfg.Interval)
		breaker.advance(now)
	}
}
func BenchmarkTransitions(b *testing.B) {
	breaker := benchBreaker(b)
	now := time.Now()
	b.ReportAllocs()
	for b.Loop() {
		breaker.start(StateOpen, now)
		breaker.advance(now.Add(breaker.cfg.OpenTimeout))
		breaker.start(StateClosed, now)
	}
}
func BenchmarkConcurrent(b *testing.B) {
	breaker := benchBreaker(b)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v, err := Execute(context.Background(), breaker, func(context.Context) (int, error) { return 42, nil })
			if v != 42 || err != nil {
				b.Fatal(v, err)
			}
		}
	})
}
func BenchmarkIndependent(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		breaker := benchBreaker(b)
		for pb.Next() {
			v, err := Execute(context.Background(), breaker, func(context.Context) (int, error) { return 42, nil })
			if v != 42 || err != nil {
				b.Fatal(v, err)
			}
		}
	})
}
