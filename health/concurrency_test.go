package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type panicPayload struct{ Secret string }

func (*panicPayload) String() string { panic("panic value was formatted") }
func TestPanicAndGoexitPrivateAndRecoverable(t *testing.T) {
	for _, mode := range []string{"panic", "nil-panic", "goexit"} {
		t.Run(mode, func(t *testing.T) {
			r := newRegistry(t, DefaultConfig())
			token := &panicPayload{Secret: "private-panic-payload"}
			var calls atomic.Int64
			register(t, r, "checker", CheckerFunc(func(context.Context) error {
				if calls.Add(1) > 1 {
					return nil
				}
				switch mode {
				case "panic":
					panic(token)
				case "nil-panic":
					panic(nil)
				default:
					runtime.Goexit()
				}
				return nil
			}))
			report, err := r.Check(context.Background())
			if err != nil || report.Ready || report.Checks[0].Reason != "panic" {
				t.Fatal("panic diagnostic")
			}
			var pe *PanicError
			if !errors.Is(report.Checks[0].Err, ErrPanic) || !errors.As(report.Checks[0].Err, &pe) {
				t.Fatal("panic sentinel/type")
			}
			if pe.Error() != ErrPanic.Error() || pe.Goexit != (mode == "goexit") || len(pe.Stack) == 0 || !bytes.Contains(pe.Stack, []byte("goroutine")) {
				t.Fatal("panic shape/stack")
			}
			if mode == "panic" && pe.Value != token {
				t.Fatal("panic identity")
			}
			if mode == "goexit" && pe.Value != nil {
				t.Fatal("goexit value")
			}
			raw, err := json.Marshal(report)
			if err != nil || strings.Contains(string(raw), "Value") || strings.Contains(string(raw), "Stack") || strings.Contains(string(raw), token.Secret) || strings.Contains(string(raw), "goroutine") {
				t.Fatal("panic data leaked", string(raw))
			}
			waitIdle(r)
			next, err := r.Ready(context.Background())
			if err != nil || !next.Ready {
				t.Fatal("slot not released")
			}
			waitIdle(r)
		})
	}
}
func TestStuckCheckGoroutinesBoundedAcrossPolls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := Config{time.Second, 2, 2}
		r := newRegistry(t, cfg)
		gate := make(chan struct{})
		var calls atomic.Int64
		for _, name := range []string{"one", "two"} {
			register(t, r, name, CheckerFunc(func(context.Context) error { calls.Add(1); <-gate; return nil }))
		}
		before := runtime.NumGoroutine()
		first, err := r.Check(context.Background())
		if err != nil || first.Ready {
			t.Fatal("initial timeout")
		}
		synctest.Wait()
		blocked := runtime.NumGoroutine()
		if blocked > before+cfg.MaxParallel {
			t.Fatalf("owned goroutine bound before=%d blocked=%d", before, blocked)
		}
		for range 1000 {
			report, err := r.Check(context.Background())
			if err != nil || report.Ready || report.Checks[0].Reason != "busy" || report.Checks[1].Reason != "busy" {
				t.Fatal("busy poll")
			}
		}
		synctest.Wait()
		if calls.Load() != 2 || runtime.NumGoroutine() > blocked {
			t.Fatal("polls spawned replacement goroutines")
		}
		close(gate)
		waitIdle(r)
		synctest.Wait()
		report, err := r.Ready(context.Background())
		if err != nil || !report.Ready || calls.Load() != 4 {
			t.Fatal("fresh result after old completion")
		}
		waitIdle(r)
		synctest.Wait()
	})
}
func TestConcurrentEvaluationsShareGlobalSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRegistry(t, Config{time.Second, 2, 8})
		gate := make(chan struct{})
		var live, peak atomic.Int64
		for i := range 8 {
			register(t, r, string(rune('a'+i)), CheckerFunc(func(context.Context) error {
				current := live.Add(1)
				defer live.Add(-1)
				for {
					old := peak.Load()
					if current <= old || peak.CompareAndSwap(old, current) {
						break
					}
				}
				<-gate
				return nil
			}))
		}
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				report, err := r.Check(context.Background())
				if err != nil || len(report.Checks) != 8 || report.Ready {
					t.Error("concurrent diagnostic")
				}
			})
		}
		wg.Wait()
		if peak.Load() > 2 || live.Load() != 2 {
			t.Fatal("global slots exceeded")
		}
		close(gate)
		waitIdle(r)
		synctest.Wait()
		if live.Load() != 0 {
			t.Fatal("slot leak")
		}
	})
}
func TestSuccessBeforeOtherTimeoutRemainsUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRegistry(t, Config{time.Second, 1, 2})
		register(t, r, "fast", CheckerFunc(func(context.Context) error { return nil }))
		register(t, r, "slow", CheckerFunc(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }))
		report, err := r.Check(context.Background())
		if err != nil || report.Checks[0].Status != "up" || report.Checks[1].Reason != "timeout" {
			t.Fatal("known result lost at deadline")
		}
		waitIdle(r)
	})
}
