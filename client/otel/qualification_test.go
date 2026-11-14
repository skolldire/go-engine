package otel

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/skolldire/go-engine/client"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func noopDependencies(mask byte) Dependencies {
	d := Dependencies{}
	if mask&1 != 0 {
		d.TracerProvider = tracenoop.NewTracerProvider()
	}
	if mask&2 != 0 {
		d.MeterProvider = metricnoop.NewMeterProvider()
	}
	return d
}
func checkLeaks(t testing.TB) {
	t.Helper()
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Fatal("standard goroutineleak profile missing")
	}
	var out bytes.Buffer
	if e := p.WriteTo(&out, 1); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "total 0") {
		t.Fatal(out.String())
	}
	t.Log(out.String())
}
func TestNoGoroutineLeaks(t *testing.T) {
	mw := makeMiddleware[int](t, noopDependencies(3))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 1, nil }, mw)
			}
		})
	}
	wg.Wait()
	checkLeaks(t)
}
func TestStressConcurrentSignals(t *testing.T) {
	if os.Getenv("CLIENTOTEL_STRESS") != "1" && os.Getenv("CLIENTOTELSTRESS") != "1" {
		t.Skip("expanded campaign")
	}
	for _, mask := range []byte{1, 2, 3} {
		mw := makeMiddleware[int](t, noopDependencies(mask))
		var wg sync.WaitGroup
		var count atomic.Int64
		for range 16 {
			wg.Go(func() {
				for i := range 2000 {
					ctx, cancel := context.WithCancel(context.Background())
					if i%3 == 0 {
						cancel()
					}
					v, e := client.Do(ctx, invocation(), func(context.Context) (int, error) {
						count.Add(1)
						if i%2 == 0 {
							return 42, context.Canceled
						}
						return 42, nil
					}, mw)
					cancel()
					if v != 42 || (i%2 == 0 && !errors.Is(e, context.Canceled)) {
						t.Error("result/error invariant")
					}
				}
			})
		}
		wg.Wait()
		if count.Load() != 32000 {
			t.Fatal("attempt count")
		}
	}
	checkLeaks(t)
}
func TestStressAbnormalAndReleasedClosures(t *testing.T) {
	if os.Getenv("CLIENTOTEL_STRESS") != "1" {
		t.Skip("expanded campaign")
	}
	for range 200 {
		tp, mp := newTraceCapture(), newMetricCapture()
		mw := makeMiddleware[int](t, Dependencies{tp, mp, nil})
		func() {
			defer func() {
				if recover() == nil {
					t.Error("panic missing")
				}
			}()
			_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { panic("caller panic") }, mw)
		}()
		if tp.spans[0].ends.Load() != 1 || len(mp.measurements) != 0 {
			t.Fatal("abnormal completion")
		}
	}
	batch := func() {
		for range 1000 {
			mw := makeMiddleware[int](t, noopDependencies(3))
			payload := make([]byte, 16<<10)
			terminal := func(context.Context, client.Invocation, client.Operation[int]) (int, error) { return len(payload), nil }
			handler, e := client.Chain(terminal, mw)
			if e != nil {
				t.Fatal(e)
			}
			v, e := handler(context.Background(), invocation(), nil)
			if v != len(payload) || e != nil {
				t.Fatal("fixture")
			}
		}
	}
	batch()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 3 {
		batch()
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	if after.HeapAlloc > before.HeapAlloc+8<<20 {
		t.Fatalf("retention before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
	}
	t.Logf("4,000 abandoned 16KiB fixtures: before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
	checkLeaks(t)
}
func FuzzConfig(f *testing.F) {
	f.Add("instrumentation", byte(3), false)
	f.Fuzz(func(t *testing.T, name string, mask byte, classifier bool) {
		if len(name) > 1024 {
			t.Skip()
		}
		d := noopDependencies(mask % 4)
		if classifier {
			d.IsFailure = func(error) bool { return true }
		}
		mw, e := New[int](Config{name}, d)
		invalid := name == "" || mask%4 == 0 || (classifier && mask%4 == 2)
		if invalid {
			if !errors.Is(e, ErrInvalidConfig) || mw != nil {
				t.Fatal("invalid accepted")
			}
		} else if e != nil || mw == nil {
			t.Fatal("valid rejected")
		}
	})
}
func FuzzLabelsAndResults(f *testing.F) {
	f.Add("storage", "read", byte(3), true, int64(42))
	f.Fuzz(func(t *testing.T, component, operation string, mask byte, failed bool, value int64) {
		if len(component) > 129 || len(operation) > 129 {
			t.Skip()
		}
		d := noopDependencies(mask%3 + 1)
		mw := makeMiddleware[int64](t, d)
		var original error
		if failed {
			original = secretError{}
		}
		calls := 0
		v, e := client.Do(context.Background(), client.Invocation{Component: component, Operation: operation}, func(context.Context) (int64, error) { calls++; return value, original }, mw)
		valid := len(component) > 0 && len(component) <= 128 && len(operation) > 0 && len(operation) <= 128
		if !valid {
			if calls != 0 || !errors.Is(e, client.ErrInvalidCall) {
				t.Fatal("invalid invocation")
			}
		} else if calls != 1 || v != value || e != original {
			t.Fatal("result/error invariant")
		}
	})
}
func FuzzReuseSequence(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3})
	f.Fuzz(func(t *testing.T, seq []byte) {
		if len(seq) > 64 {
			t.Skip()
		}
		tp, mp := newTraceCapture(), newMetricCapture()
		mw := makeMiddleware[int](t, Dependencies{tp, mp, func(error) bool { return true }})
		for _, op := range seq {
			ctx, cancel := context.WithCancel(context.Background())
			if op&1 != 0 {
				cancel()
			}
			_, _ = client.Do(ctx, invocation(), func(context.Context) (int, error) {
				if op&2 != 0 {
					return 7, secretError{}
				}
				return 7, nil
			}, mw)
			cancel()
		}
		if len(tp.spans) != len(seq) || len(mp.measurements) != len(seq) {
			t.Fatal("reuse count")
		}
		for _, s := range tp.spans {
			if s.ends.Load() != 1 || s.recordErrors.Load() != 0 {
				t.Fatal("finalization")
			}
		}
	})
}
func BenchmarkDirect(b *testing.B) {
	op := func(context.Context) (int, error) { return 42, nil }
	b.ReportAllocs()
	for b.Loop() {
		_, _ = op(context.Background())
	}
}
func BenchmarkDoWithoutMiddleware(b *testing.B) {
	op := func(context.Context) (int, error) { return 42, nil }
	b.ReportAllocs()
	for b.Loop() {
		_, _ = client.Do(context.Background(), invocation(), op)
	}
}
func BenchmarkConstruction(b *testing.B) {
	d := noopDependencies(3)
	b.ReportAllocs()
	for b.Loop() {
		if _, e := New[int](Config{"benchmark"}, d); e != nil {
			b.Fatal(e)
		}
	}
}
func BenchmarkSignals(b *testing.B) {
	for _, mask := range []byte{1, 2, 3} {
		b.Run(map[byte]string{1: "trace", 2: "metric", 3: "both"}[mask], func(b *testing.B) {
			mw := makeMiddleware[int](b, noopDependencies(mask))
			op := func(context.Context) (int, error) { return 42, nil }
			handler, e := client.Chain(func(ctx context.Context, _ client.Invocation, op client.Operation[int]) (int, error) { return op(ctx) }, mw)
			if e != nil {
				b.Fatal(e)
			}
			b.ReportAllocs()
			for b.Loop() {
				_, _ = handler(context.Background(), invocation(), op)
			}
		})
	}
}
func BenchmarkDoConstructingChain(b *testing.B) {
	mw := makeMiddleware[int](b, noopDependencies(3))
	op := func(context.Context) (int, error) { return 42, nil }
	b.ReportAllocs()
	for b.Loop() {
		_, _ = client.Do(context.Background(), invocation(), op, mw)
	}
}
func BenchmarkStructResult(b *testing.B) {
	type value struct{ A, B, C int }
	mw := makeMiddleware[value](b, noopDependencies(3))
	op := func(context.Context) (value, error) { return value{1, 2, 3}, nil }
	handler, e := client.Chain(func(ctx context.Context, _ client.Invocation, op client.Operation[value]) (value, error) {
		return op(ctx)
	}, mw)
	if e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = handler(context.Background(), invocation(), op)
	}
}
func BenchmarkParallel(b *testing.B) {
	mw := makeMiddleware[int](b, noopDependencies(3))
	op := func(context.Context) (int, error) { return 42, nil }
	handler, e := client.Chain(func(ctx context.Context, _ client.Invocation, op client.Operation[int]) (int, error) { return op(ctx) }, mw)
	if e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = handler(context.Background(), invocation(), op)
		}
	})
}

func BenchmarkChainWithoutMiddleware(b *testing.B) {
	op := func(context.Context) (int, error) { return 42, nil }
	handler, e := client.Chain(func(ctx context.Context, _ client.Invocation, op client.Operation[int]) (int, error) { return op(ctx) })
	if e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = handler(context.Background(), invocation(), op)
	}
}
func BenchmarkIndependentParallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		mw, e := New[int](Config{"independent"}, noopDependencies(3))
		if e != nil {
			b.Error(e)
			return
		}
		op := func(context.Context) (int, error) { return 42, nil }
		handler, e := client.Chain(func(ctx context.Context, _ client.Invocation, op client.Operation[int]) (int, error) { return op(ctx) }, mw)
		if e != nil {
			b.Error(e)
			return
		}
		for pb.Next() {
			_, _ = handler(context.Background(), invocation(), op)
		}
	})
}
