package retry

import (
	"context"
	"errors"
	"math/rand/v2"
	"testing"
)

var benchmarkValue int
var benchmarkError error

func BenchmarkDirect(b *testing.B) {
	op := func(context.Context) (int, error) { return 42, nil }
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		benchmarkValue, benchmarkError = op(ctx)
	}
}
func BenchmarkDoSuccess(b *testing.B) {
	op := func(context.Context) (int, error) { return 42, nil }
	cfg := DefaultConfig()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		benchmarkValue, benchmarkError = Do(ctx, cfg, nil, op)
	}
}
func BenchmarkClassified(b *testing.B) {
	original := errors.New("failure")
	for _, decision := range []Decision{DecisionStop, DecisionReconcile} {
		b.Run(string(decision), func(b *testing.B) {
			cfg := DefaultConfig()
			op := func(context.Context) (int, error) { return 42, original }
			classify := func(error) Decision { return decision }
			b.ReportAllocs()
			for b.Loop() {
				benchmarkValue, benchmarkError = Do(context.Background(), cfg, classify, op)
			}
		})
	}
}
func BenchmarkBackoff(b *testing.B) {
	cfg := DefaultConfig()
	b.ReportAllocs()
	for b.Loop() {
		benchmarkValue = int(backoff(cfg, 5))
	}
}
func BenchmarkJitter(b *testing.B) {
	cfg := DefaultConfig()
	b.ReportAllocs()
	for b.Loop() {
		benchmarkValue = int(fullJitter(cfg.MaxBackoff, rand.Uint64N))
	}
}
func BenchmarkRetries(b *testing.B) {
	original := errors.New("transient")
	for _, success := range []bool{true, false} {
		name := "exhausted"
		if success {
			name = "success"
		}
		b.Run(name, func(b *testing.B) {
			cfg := fastConfig()
			cfg.MaxRetries = 2
			calls := 0
			op := func(context.Context) (int, error) {
				calls++
				if success && calls == 3 {
					return 42, nil
				}
				return 42, original
			}
			b.ReportAllocs()
			for b.Loop() {
				calls = 0
				benchmarkValue, benchmarkError = Do(context.Background(), cfg, retryAll, op)
			}
		})
	}
}
func BenchmarkConcurrent(b *testing.B) {
	cfg := DefaultConfig()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			value, err := Do(context.Background(), cfg, nil, func(context.Context) (int, error) { return 42, nil })
			if value != 42 || err != nil {
				b.Fatal(value, err)
			}
		}
	})
}
