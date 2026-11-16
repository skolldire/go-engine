package health

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func checkLeaks(t testing.TB) {
	t.Helper()
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Fatal("standard goroutineleak profile missing")
	}
	var report bytes.Buffer
	if err := p.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.String(), "total 0") {
		t.Fatal(report.String())
	}
	t.Log(report.String())
}
func TestNoGoroutineLeaks(t *testing.T) {
	r := newRegistry(t, DefaultConfig())
	register(t, r, "check", CheckerFunc(func(context.Context) error { return nil }))
	for range 100 {
		_, err := r.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		waitIdle(r)
	}
	checkLeaks(t)
}
func TestStressConcurrentEvaluationAndAbnormal(t *testing.T) {
	if os.Getenv("HEALTH_STRESS") != "1" {
		t.Skip("expanded campaign")
	}
	r := newRegistry(t, Config{time.Second, 4, 8})
	var calls atomic.Int64
	for i := range 8 {
		register(t, r, fmt.Sprint("check", i), CheckerFunc(func(ctx context.Context) error {
			switch calls.Add(1) % 5 {
			case 0:
				panic("private panic")
			case 1:
				runtime.Goexit()
			case 2:
				return errors.New("private checker error")
			case 3:
				return ctx.Err()
			default:
				return nil
			}
			return nil
		}))
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 200 {
				report, err := r.Check(context.Background())
				if err != nil || len(report.Checks) != 8 {
					t.Error("diagnostic")
				}
				for _, result := range report.Checks {
					if result.Status != "up" && result.Status != "down" {
						t.Error("status")
					}
				}
			}
		})
	}
	wg.Wait()
	waitIdle(r)
	t.Logf("3200 concurrent evaluations, %d actual checker executions", calls.Load())
	checkLeaks(t)
}
func TestStressStuckAndCanceled(t *testing.T) {
	if os.Getenv("HEALTH_STRESS") != "1" {
		t.Skip("expanded campaign")
	}
	synctest.Test(t, func(t *testing.T) {
		r := newRegistry(t, Config{time.Second, 2, 2})
		gate := make(chan struct{})
		for _, name := range []string{"a", "b"} {
			register(t, r, name, CheckerFunc(func(context.Context) error { <-gate; return nil }))
		}
		_, _ = r.Check(context.Background())
		for range 2000 {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			report, err := r.Check(ctx)
			if err != nil || report.Checks[0].Reason != "busy" {
				t.Fatal("stuck poll")
			}
		}
		close(gate)
		waitIdle(r)
		synctest.Wait()
	})
	checkLeaks(t)
}
func TestStressReleasedRegistries(t *testing.T) {
	if os.Getenv("HEALTH_STRESS") != "1" {
		t.Skip("expanded campaign")
	}
	batch := func() {
		for i := range 250 {
			r := newRegistry(t, DefaultConfig())
			register(t, r, "check", CheckerFunc(func(context.Context) error {
				if i%2 == 0 {
					panic("private released panic stack")
				}
				return nil
			}))
			_, err := r.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			waitIdle(r)
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
		t.Fatalf("registry fixture retained heap: before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
	}
	t.Logf("1000 closed-evaluation registry fixtures heap before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
	checkLeaks(t)
}
func FuzzConfig(f *testing.F) {
	f.Add(int64(time.Second), 4, 128)
	f.Fuzz(func(t *testing.T, timeout int64, parallel, count int) {
		cfg := Config{time.Duration(timeout), parallel, count}
		copy := cfg
		err := cfg.Validate()
		if (err == nil) != (timeout > 0 && parallel > 0 && count > 0) || cfg != copy {
			t.Fatal("config validation/mutation")
		}
		if err == nil {
			r, e := New(cfg)
			if e != nil || !r.Live() {
				t.Fatal("constructor")
			}
		}
	})
}
func FuzzNames(f *testing.F) {
	f.Add("db")
	f.Add("bad/name")
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 1024 {
			t.Skip()
		}
		r := newRegistry(t, DefaultConfig())
		err := r.Register(name, CheckerFunc(func(context.Context) error { return nil }))
		valid := len(name) > 0 && len(name) <= 128
		for _, b := range []byte(name) {
			if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.-", rune(b)) {
				valid = false
			}
		}
		if (err == nil) != valid {
			t.Fatal("name validation")
		}
		if !valid && !errors.Is(err, ErrInvalidCall) {
			t.Fatal("name sentinel")
		}
	})
}
func FuzzRegistrySequence(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 0})
	f.Fuzz(func(t *testing.T, sequence []byte) {
		if len(sequence) > 32 {
			t.Skip()
		}
		r := newRegistry(t, Config{time.Second, 2, 4})
		frozen := false
		registered := 0
		for _, op := range sequence {
			switch op % 3 {
			case 0:
				err := r.Register(fmt.Sprint("check", registered), CheckerFunc(func(context.Context) error { return nil }))
				if frozen {
					if !errors.Is(err, ErrRegistryFrozen) {
						t.Fatal("freeze")
					}
				} else if registered >= 4 {
					if !errors.Is(err, ErrTooManyChecks) {
						t.Fatal("limit")
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					registered++
				}
			default:
				frozen = true
				var report Report
				var err error
				if op%3 == 1 {
					report, err = r.Ready(context.Background())
				} else {
					report, err = r.Check(context.Background())
				}
				if err != nil || !report.Ready || len(report.Checks) != registered {
					t.Fatal("sequence report")
				}
				waitIdle(r)
			}
		}
	})
}
func BenchmarkEvaluation(b *testing.B) {
	for _, count := range []int{1, 16, 128} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			r := newRegistry(b, DefaultConfig())
			for i := range count {
				register(b, r, fmt.Sprint("check", i), CheckerFunc(func(context.Context) error { return nil }))
			}
			b.ReportAllocs()
			for b.Loop() {
				report, err := r.Check(context.Background())
				if err != nil || !report.Ready {
					b.Fatal(err)
				}
				waitIdle(r)
			}
		})
	}
}
func BenchmarkReportConstruction(b *testing.B) {
	for _, count := range []int{1, 16, 128} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				report := Report{Live: true, Ready: true, Checks: make([]CheckResult, count)}
				for i := range report.Checks {
					report.Checks[i] = CheckResult{Status: "up", Reason: "ok"}
				}
			}
		})
	}
}
