package logging_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/skolldire/go-engine/client"
	clientlogging "github.com/skolldire/go-engine/client/logging"
)

type counterLogger struct {
	enabled   bool
	emissions atomic.Int64
}

func (l *counterLogger) Enabled(context.Context, slog.Level) bool { return l.enabled }
func (l *counterLogger) LogAttrs(context.Context, slog.Level, string, ...slog.Attr) {
	l.emissions.Add(1)
}
func assertNoLeaks(t *testing.T) {
	t.Helper()
	runtime.GC()
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Fatal("goroutineleak profile absent")
	}
	var out bytes.Buffer
	if err := p.WriteTo(&out, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "total 0") {
		t.Fatal(out.String())
	}
}
func TestNoGoroutineLeaks(t *testing.T) {
	logger := &counterLogger{enabled: true}
	mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			for range 30 {
				_, err := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 1, nil }, mw)
				if err != nil {
					t.Error("unexpected error")
				}
			}
		})
	}
	wg.Wait()
	if logger.emissions.Load() != 360 {
		t.Fatal(logger.emissions.Load())
	}
	assertNoLeaks(t)
}
func stressOnly(t *testing.T) {
	t.Helper()
	if os.Getenv("CLIENTLOGGING_STRESS") == "" && os.Getenv("CLIENTLOGGINGSTRESS") == "" {
		t.Skip("set CLIENTLOGGING_STRESS=1")
	}
}
func TestStressConcurrentChains(t *testing.T) {
	stressOnly(t)
	logger := &sliceLogger{enabled: true}
	mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
	chain, err := client.Chain(client.Handler[int](terminal[int]), mw)
	if err != nil {
		t.Fatal(err)
	}
	const workers, rounds = 24, 400
	var wg sync.WaitGroup
	var calls atomic.Int64
	failure := &domainError{}
	for worker := range workers {
		wg.Go(func() {
			for seq := range rounds {
				ctx, cancel := context.WithCancel(context.Background())
				if seq%3 == 0 {
					cancel()
				}
				value, err := chain(ctx, invocation(), func(context.Context) (int, error) {
					calls.Add(1)
					if seq%2 == 0 {
						return worker, failure
					}
					return worker, nil
				})
				cancel()
				if value != worker || (seq%2 == 0 && err != failure) || (seq%2 != 0 && err != nil) {
					t.Error("result changed")
				}
			}
		})
	}
	wg.Wait()
	events := logger.snapshot()
	if calls.Load() != workers*rounds || len(events) != workers*rounds {
		t.Fatal("lost/duplicate calls")
	}
	failed := 0
	for _, event := range events {
		if attrs(event)["success"] == false {
			failed++
		}
	}
	if failed != workers*rounds/2 {
		t.Fatal("incorrect outcomes")
	}
	assertNoLeaks(t)
	t.Logf("%d callers, %d completions, %d errors unchanged", workers, len(events), failed)
}
func TestStressDisabledAndIndependent(t *testing.T) {
	stressOnly(t)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			logger := &counterLogger{}
			mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
			for range 500 {
				_, err := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 1, nil }, mw)
				if err != nil {
					t.Error("unexpected error")
				}
			}
			if logger.emissions.Load() != 0 {
				t.Error("disabled emitted")
			}
		})
	}
	wg.Wait()
	assertNoLeaks(t)
}
func TestStressRetention(t *testing.T) {
	stressOnly(t)
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	baseline := memory.HeapAlloc
	logger := &counterLogger{enabled: true}
	mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
	for range 8 {
		for range 100 {
			payload := strings.Repeat("x", 64<<10)
			ctx := context.WithValue(context.Background(), contextKey{}, payload)
			op := func(got context.Context) (int, error) { return len(got.Value(contextKey{}).(string)), nil }
			chain, err := client.Chain(client.Handler[int](terminal[int]), mw)
			if err != nil {
				t.Fatal(err)
			}
			if value, err := chain(ctx, invocation(), op); err != nil || value != 64<<10 {
				t.Fatal("fixture failed")
			}
		}
		runtime.GC()
		runtime.ReadMemStats(&memory)
		if memory.HeapAlloc > baseline+(8<<20) {
			t.Fatalf("retained fixture memory baseline=%d current=%d", baseline, memory.HeapAlloc)
		}
	}
	runtime.KeepAlive(mw)
	assertNoLeaks(t)
	t.Logf("800 completed chains with 64KiB contexts: baseline=%d final=%d allowance=8MiB", baseline, memory.HeapAlloc)
}
func FuzzConfiguration(f *testing.F) {
	f.Add("debug", true, uint8(0))
	f.Add("info", false, uint8(1))
	f.Add("invalid", false, uint8(2))
	f.Fuzz(func(t *testing.T, level string, duration bool, kind uint8) {
		if len(level) > 4096 {
			return
		}
		var logger clientlogging.Logger = &counterLogger{}
		if kind%3 == 1 {
			logger = nil
		}
		if kind%3 == 2 {
			logger = (*slog.Logger)(nil)
		}
		mw, err := clientlogging.New[int](clientlogging.Config{Level: level, IncludeDuration: duration}, clientlogging.Dependencies{Logger: logger})
		if level != "debug" && level != "info" {
			var ce *clientlogging.ConfigError
			if !errors.As(err, &ce) || ce.Field != "Level" {
				t.Fatal("validation order")
			}
			return
		}
		valid := kind%3 == 0
		if valid {
			if err != nil || mw == nil {
				t.Fatal("valid config rejected")
			}
		} else if !errors.Is(err, clientlogging.ErrInvalidConfig) {
			t.Fatal("nil logger accepted")
		}
	})
}
func FuzzInvocation(f *testing.F) {
	f.Add("component", "operation", true, true)
	f.Add("", "", false, false)
	f.Add(string([]byte{0xff}), "read", true, false)
	f.Fuzz(func(t *testing.T, component, operation string, enabled, duration bool) {
		if len(component)+len(operation) > 1024 {
			return
		}
		logger := &sliceLogger{enabled: enabled}
		cfg := clientlogging.DefaultConfig()
		cfg.IncludeDuration = duration
		mw := newMiddleware[int](t, cfg, logger)
		calls := 0
		value, err := client.Do(context.Background(), client.Invocation{Component: component, Operation: operation}, func(context.Context) (int, error) { calls++; return 7, nil }, mw)
		valid := len(component) > 0 && len(component) <= 128 && len(operation) > 0 && len(operation) <= 128
		if !valid {
			if !errors.Is(err, client.ErrInvalidCall) || calls != 0 || len(logger.snapshot()) != 0 {
				t.Fatal("invalid call observed")
			}
			return
		}
		if value != 7 || err != nil || calls != 1 {
			t.Fatal("valid call changed")
		}
		events := logger.snapshot()
		if !enabled {
			if len(events) != 0 {
				t.Fatal("disabled emitted")
			}
			return
		}
		if len(events) != 1 {
			t.Fatal("emission count")
		}
		fields := attrs(events[0])
		if fields["component"] != component || fields["operation"] != operation {
			t.Fatal("labels rewritten")
		}
		_, hasDuration := fields["duration"]
		if hasDuration != duration {
			t.Fatal("duration presence")
		}
	})
}
func FuzzSequences(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Add([]byte{255, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64 {
			return
		}
		logger := &sliceLogger{enabled: true}
		mw := newMiddleware[privateResult](t, clientlogging.DefaultConfig(), logger)
		chain, err := client.Chain(client.Handler[privateResult](terminal[privateResult]), mw)
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range data {
			ctx, cancel := context.WithCancel(context.Background())
			if v%3 == 0 {
				cancel()
			}
			failure := &domainError{}
			want := privateResult{int(v)}
			var expected error
			if v%2 == 0 {
				expected = failure
			}
			result, err := chain(ctx, invocation(), func(context.Context) (privateResult, error) { return want, expected })
			cancel()
			if result != want || err != expected {
				t.Fatal("result/error changed")
			}
			events := logger.snapshot()
			if len(events) != i+1 || attrs(events[i])["success"] != (expected == nil) {
				t.Fatal("sequence observation changed")
			}
		}
	})
}
func FuzzComposition(f *testing.F) {
	f.Add(uint8(1), true)
	f.Add(uint8(4), false)
	f.Fuzz(func(t *testing.T, size uint8, outer bool) {
		count := int(size % 16)
		calls, terminals := 0, 0
		logger := &counterLogger{enabled: true}
		observe := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
		middleware := make([]client.Middleware[int], count)
		for i := range middleware {
			middleware[i] = func(next client.Handler[int]) client.Handler[int] {
				return func(ctx context.Context, info client.Invocation, op client.Operation[int]) (int, error) {
					calls++
					return next(ctx, info, op)
				}
			}
		}
		if outer {
			middleware = append([]client.Middleware[int]{observe}, middleware...)
		} else {
			middleware = append(middleware, observe)
		}
		value, err := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { terminals++; return 9, nil }, middleware...)
		if err != nil || value != 9 || terminals != 1 || calls != count || logger.emissions.Load() != 1 {
			t.Fatal("composition changed")
		}
	})
}

type discardLogger struct{ enabled bool }

func (l discardLogger) Enabled(context.Context, slog.Level) bool                 { return l.enabled }
func (discardLogger) LogAttrs(context.Context, slog.Level, string, ...slog.Attr) {}
func BenchmarkExecution(b *testing.B) {
	ctx := context.Background()
	info := invocation()
	op := client.Operation[int](func(context.Context) (int, error) { return 1, nil })
	b.Run("operation", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			value, err := op(ctx)
			if value != 1 || err != nil {
				b.Fatal("fixture")
			}
		}
	})
	b.Run("client-do", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, err := client.Do(ctx, info, op)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	base, err := client.Chain(client.Handler[int](terminal[int]))
	if err != nil {
		b.Fatal(err)
	}
	b.Run("terminal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, err := base(ctx, info, op)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	for _, tc := range []struct {
		name              string
		enabled, duration bool
	}{{"disabled", false, true}, {"enabled", true, false}, {"duration", true, true}} {
		cfg := clientlogging.DefaultConfig()
		cfg.IncludeDuration = tc.duration
		mw := newMiddleware[int](b, cfg, discardLogger{tc.enabled})
		chain, err := client.Chain(client.Handler[int](terminal[int]), mw)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(tc.name+"/chain", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := chain(ctx, info, op)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(tc.name+"/do", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := client.Do(ctx, info, op, mw)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	b.Run("construction", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mw, err := clientlogging.New[int](clientlogging.DefaultConfig(), clientlogging.Dependencies{Logger: discardLogger{true}})
			if err != nil || mw == nil {
				b.Fatal("construction failed")
			}
		}
	})
}
func BenchmarkBackendsAndParallel(b *testing.B) {
	for _, tc := range []struct {
		name   string
		logger clientlogging.Logger
	}{
		{"own", discardLogger{true}},
		{"slog-discard", slog.New(slog.DiscardHandler)},
		{"json", slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))},
	} {
		b.Run(tc.name, func(b *testing.B) {
			mw := newMiddleware[int](b, clientlogging.DefaultConfig(), tc.logger)
			op := client.Operation[int](func(context.Context) (int, error) { return 1, nil })
			chain, err := client.Chain(client.Handler[int](terminal[int]), mw)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				_, err := chain(context.Background(), invocation(), op)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	b.Run("struct", func(b *testing.B) {
		mw := newMiddleware[privateResult](b, clientlogging.DefaultConfig(), discardLogger{true})
		op := client.Operation[privateResult](func(context.Context) (privateResult, error) { return privateResult{1}, nil })
		b.ReportAllocs()
		for b.Loop() {
			_, err := client.Do(context.Background(), invocation(), op, mw)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("shared-json", func(b *testing.B) {
		logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
		mw := newMiddleware[int](b, clientlogging.DefaultConfig(), logger)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 1, nil }, mw)
				if err != nil {
					b.Error(err)
				}
			}
		})
	})
	b.Run("independent-json", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
			mw := newMiddleware[int](b, clientlogging.DefaultConfig(), logger)
			for pb.Next() {
				_, err := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 1, nil }, mw)
				if err != nil {
					b.Error(err)
				}
			}
		})
	})
}
