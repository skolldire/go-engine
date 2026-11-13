package ratelimit

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

var benchmarkAllowed bool
var benchmarkError error

func benchLimiter(b *testing.B) *Limiter {
	b.Helper()
	l, err := New(Config{math.SmallestNonzeroFloat64, math.MaxInt})
	if err != nil {
		b.Fatal(err)
	}
	return l
}
func BenchmarkAllowSuccess(b *testing.B) {
	l := benchLimiter(b)
	b.ReportAllocs()
	for b.Loop() {
		benchmarkAllowed = l.Allow(1)
	}
}
func BenchmarkAllowRejected(b *testing.B) {
	l := benchLimiter(b)
	l.Allow(math.MaxInt)
	b.ReportAllocs()
	for b.Loop() {
		benchmarkAllowed = l.Allow(1)
	}
}
func BenchmarkWaitImmediate(b *testing.B) {
	l := benchLimiter(b)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		benchmarkError = l.Wait(ctx, 1)
	}
}
func BenchmarkInvalid(b *testing.B) {
	l := benchLimiter(b)
	b.ReportAllocs()
	for b.Loop() {
		benchmarkError = l.Wait(context.Background(), 0)
	}
}
func BenchmarkRefill(b *testing.B) {
	for _, saturate := range []bool{false, true} {
		name := "partial"
		if saturate {
			name = "saturated"
		}
		b.Run(name, func(b *testing.B) {
			l, err := New(Config{1000, math.MaxInt})
			if err != nil {
				b.Fatal(err)
			}
			l.tokens = 0
			now := l.anchor.at
			b.ReportAllocs()
			for b.Loop() {
				if saturate {
					l.tokens = l.cfg.Burst - 1
					now = now.Add(time.Hour)
				} else {
					now = now.Add(time.Nanosecond)
				}
				l.refill(now)
			}
		})
	}
}

type blockedContext struct {
	context.Context
	calls    int
	observed chan struct{}
	once     sync.Once
}

func (c *blockedContext) Done() <-chan struct{} {
	c.calls++
	if c.calls >= 2 {
		c.once.Do(func() { close(c.observed) })
	}
	return c.Context.Done()
}
func BenchmarkQueueCancellation(b *testing.B) {
	l, err := New(Config{math.SmallestNonzeroFloat64, 1})
	if err != nil {
		b.Fatal(err)
	}
	l.Allow(1)
	b.ReportAllocs()
	for b.Loop() {
		parent, cancel := context.WithCancel(context.Background())
		ctx := &blockedContext{Context: parent, observed: make(chan struct{})}
		done := make(chan error, 1)
		go func() { done <- l.Wait(ctx, 1) }()
		<-ctx.observed
		cancel()
		benchmarkError = <-done
	}
}
func BenchmarkConcurrent(b *testing.B) {
	l := benchLimiter(b)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if !l.Allow(1) {
				b.Fatal("fixture exhausted")
			}
		}
	})
}
func BenchmarkIndependent(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		l := benchLimiter(b)
		for pb.Next() {
			if err := l.Wait(context.Background(), 1); err != nil {
				b.Fatal(err)
			}
		}
	})
}
