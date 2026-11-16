package health

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"

	"testing"
	"testing/synctest"
	"time"
)

func waitIdle(r *Registry) {
	for {
		r.mu.Lock()
		if r.active == 0 {
			r.mu.Unlock()
			return
		}
		changed := r.changed
		r.mu.Unlock()
		<-changed
	}
}
func TestEmptyFreezeAndNil(t *testing.T) {
	for _, nilCtx := range []bool{false, true} {
		r := newRegistry(t, DefaultConfig())
		ctx := context.Background()
		if nilCtx {
			ctx = nil
		}
		report, err := r.Ready(ctx)
		if nilCtx {
			if !errors.Is(err, ErrNilContext) {
				t.Fatal(err)
			}
		} else if err != nil || !report.Live || !report.Ready || len(report.Checks) != 0 || report.Checks == nil {
			t.Fatal(report, err)
		}
		if err := r.Register("later", CheckerFunc(func(context.Context) error { return nil })); !errors.Is(err, ErrRegistryFrozen) {
			t.Fatal(err)
		}
		//nolint:staticcheck // Nil context intentionally verifies the public invocation contract.
		if _, err := r.Check(nil); !errors.Is(err, ErrNilContext) {
			t.Fatal(err)
		}
	}
}

type privateError struct{ message string }

func (e *privateError) Error() string                { return e.message }
func (e *privateError) MarshalJSON() ([]byte, error) { return []byte(`"` + e.message + `"`), nil }
func TestReportResultsIdentityAndPrivacy(t *testing.T) {
	r := newRegistry(t, DefaultConfig())
	original := &privateError{"private-password=do-not-expose"}
	register(t, r, "first", CheckerFunc(func(context.Context) error { return nil }))
	register(t, r, "second", CheckerFunc(func(context.Context) error { return original }))
	report, err := r.Check(context.Background())
	if err != nil || !report.Live || report.Ready || len(report.Checks) != 2 {
		t.Fatal("report")
	}
	if report.Checks[0].Name != "first" || report.Checks[0].Reason != "ok" || report.Checks[1].Reason != "failed" || report.Checks[1].Status != "down" || report.Checks[1].Err != original {
		t.Fatal("ordered results/error identity")
	}
	var typed *privateError
	if !errors.Is(report.Checks[1].Err, original) || !errors.As(report.Checks[1].Err, &typed) {
		t.Fatal("cause")
	}
	raw, err := json.Marshal(report)
	if err != nil || strings.Contains(string(raw), original.message) || strings.Contains(string(raw), "Err") {
		t.Fatal("error leaked", string(raw))
	}
	report.Checks[0].Name = "mutated"
	report.Checks = append(report.Checks, CheckResult{Name: "extra"})
	waitIdle(r)
	ready, err := r.Ready(context.Background())
	if !errors.Is(err, ErrNotReady) || ready.Ready || len(ready.Checks) != 2 || ready.Checks[0].Name != "first" {
		t.Fatal("report mutated registry")
	}
	waitIdle(r)
}
func TestEvaluationDeadlineAndContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		type key struct{}
		r := newRegistry(t, Config{time.Second, 1, 4})
		start := time.Now()
		register(t, r, "slow", CheckerFunc(func(ctx context.Context) error {
			if ctx.Value(key{}) != "value" {
				t.Error("lost context value")
			}
			deadline, ok := ctx.Deadline()
			if !ok || deadline != start.Add(time.Second) {
				t.Error("missing check deadline")
			}
			<-ctx.Done()
			return ctx.Err()
		}))
		report, err := r.Check(context.WithValue(context.Background(), key{}, "value"))
		if err != nil || report.Ready || report.Checks[0].Reason != "timeout" || !errors.Is(report.Checks[0].Err, context.DeadlineExceeded) {
			t.Fatal("timeout semantics")
		}
		waitIdle(r)
	})
}
func TestCallerCanceledBeforeAdmission(t *testing.T) {
	r := newRegistry(t, DefaultConfig())
	called := false
	register(t, r, "check", CheckerFunc(func(context.Context) error { called = true; return nil }))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := r.Ready(ctx)
	if !errors.Is(err, ErrNotReady) || called || report.Checks[0].Reason != "timeout" || !errors.Is(report.Checks[0].Err, context.Canceled) {
		t.Fatal("cancel before admission")
	}
}
func TestSlotsAndStuckCheckReuse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRegistry(t, Config{time.Second, 1, 2})
		entered, gate := make(chan struct{}), make(chan struct{})
		register(t, r, "stuck", CheckerFunc(func(context.Context) error {
			select {
			case <-entered:
			default:
				close(entered)
			}
			<-gate
			return nil
		}))
		secondCalled := false
		register(t, r, "waiting", CheckerFunc(func(context.Context) error { secondCalled = true; return nil }))
		report, err := r.Check(context.Background())
		if err != nil || report.Checks[0].Reason != "timeout" || report.Checks[1].Reason != "timeout" || secondCalled {
			t.Fatal("slot timeout")
		}
		<-entered
		for range 20 {
			report, err = r.Check(context.Background())
			if err != nil || report.Checks[0].Reason != "busy" || report.Checks[1].Reason != "timeout" {
				t.Fatal("stuck reuse")
			}
		}
		close(gate)
		waitIdle(r)
		report, err = r.Ready(context.Background())
		if err != nil || !report.Ready || !secondCalled {
			t.Fatal("late result prevented recovery")
		}
		waitIdle(r)
	})
}

func TestCallerCauseAndShortDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRegistry(t, DefaultConfig())
		cause := errors.New("private caller deadline cause")
		ctx, cancel := context.WithTimeoutCause(context.Background(), 100*time.Millisecond, cause)
		defer cancel()
		seen := make(chan error, 1)
		register(t, r, "check", CheckerFunc(func(ctx context.Context) error { <-ctx.Done(); seen <- context.Cause(ctx); return ctx.Err() }))
		start := time.Now()
		report, err := r.Check(ctx)
		if err != nil || report.Checks[0].Reason != "timeout" || time.Since(start) != 100*time.Millisecond {
			t.Fatal("parent deadline not respected")
		}
		waitIdle(r)
		if <-seen != cause {
			t.Fatal("caller cause not delivered")
		}
	})
}
func TestConstructorCopiesConfigAndLiveDoesNotCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxChecks = 2
	r := newRegistry(t, cfg)
	cfg.MaxChecks = 1
	var called atomic.Bool
	for _, name := range []string{"a", "b"} {
		register(t, r, name, CheckerFunc(func(context.Context) error { called.Store(true); return nil }))
	}
	if !r.Live() || called.Load() {
		t.Fatal("Live evaluated check")
	}
	report, err := r.Check(context.Background())
	if err != nil || !report.Ready || !called.Load() {
		t.Fatal("copied config")
	}
	waitIdle(r)
}
