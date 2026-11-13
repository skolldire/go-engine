package integration

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"weak"

	"github.com/skolldire/go-engine/ratelimit"
	"github.com/skolldire/go-engine/resilience"

	"github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/retry"
)

func assertNoLeaks(t *testing.T) {
	t.Helper()
	runtime.GC()
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Fatal("goroutineleak missing")
	}
	var report bytes.Buffer
	if err := p.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if p.Count() != 0 {
		t.Fatal(report.String())
	}
	t.Log(report.String())
}
func TestNoGoroutineLeaks(t *testing.T) {
	cfg := config()
	cfg.TotalTimeout = time.Hour
	cfg.AttemptTimeout = time.Minute
	f := wrap(t, func(context.Context) (int, error) { return 42, nil }, cfg, resilience.Dependencies{})
	for range 100 {
		if _, err := f(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	assertNoLeaks(t)
}

type invocation struct{ id, attempts int }
type invocationKey struct{}

func TestStressSharedTools(t *testing.T) {
	if os.Getenv("RESILIENCE_STRESS") != "1" {
		t.Skip("set RESILIENCE_STRESS=1")
	}
	synctest.Test(t, func(t *testing.T) {
		transient := errors.New("transient")
		business := errors.New("business")
		uncertain := errors.New("uncertain")
		l := limiter(t, 4)
		r := rate(t, 1e6, 128)
		b := breaker(t, math.MaxUint64, func(err error) bool { return errors.Is(err, transient) })
		cfg := config()
		cfg.TotalTimeout = time.Hour
		cfg.AttemptTimeout = time.Minute
		cfg.AdmissionTimeout = time.Second
		cfg.Weight = 1
		cfg.Tokens = 1
		cfg.Retry.MaxRetries = 2
		var executed atomic.Int64
		f := wrap(t, func(ctx context.Context) (int, error) {
			call := ctx.Value(invocationKey{}).(*invocation)
			call.attempts++
			executed.Add(1)
			switch call.id % 5 {
			case 1:
				if call.attempts < 3 {
					return call.id, transient
				}
			case 2:
				return call.id, business
			case 4:
				return call.id, uncertain
			}
			return call.id, nil
		}, cfg, resilience.Dependencies{Limiter: l, RateLimiter: r, Breaker: b, Classify: func(err error) retry.Decision {
			switch {
			case errors.Is(err, transient):
				return retry.DecisionRetry
			case errors.Is(err, uncertain):
				return retry.DecisionReconcile
			default:
				return retry.DecisionStop
			}
		}})
		var wg sync.WaitGroup
		for worker := range 32 {
			wg.Go(func() {
				for i := range 1000 {
					call := &invocation{id: worker*1000 + i}
					parent, cancel := context.WithCancel(context.WithValue(context.Background(), invocationKey{}, call))
					if call.id%5 == 3 {
						cancel()
					}
					value, err := f(parent)
					cancel()
					switch call.id % 5 {
					case 0:
						if err != nil || value != call.id || call.attempts != 1 {
							t.Error(value, err)
						}
					case 1:
						if err != nil || value != call.id || call.attempts != 3 {
							t.Error(value, err, call.attempts)
						}
					case 2:
						if err != business || value != call.id {
							t.Error(value, err)
						}
					case 3:
						if !errors.Is(err, context.Canceled) || call.attempts != 0 {
							t.Error(err, call.attempts)
						}
					case 4:
						if !errors.Is(err, retry.ErrReconciliationRequired) || !errors.Is(err, uncertain) || call.attempts != 1 {
							t.Error(err, call.attempts)
						}
					}
				}
			})
		}
		wg.Wait()
		if executed.Load() != 38400 {
			t.Fatal(executed.Load())
		}
		counts := b.Stats()
		if counts.Requests != 38400 || counts.InFlight != 0 || counts.Successes != 12800 || counts.Failures != 12800 || counts.Ignored != 12800 {
			t.Fatal(counts)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		release, err := l.Acquire(ctx, 4)
		if err != nil {
			t.Fatal("permits leaked", err)
		}
		release()
	})
	assertNoLeaks(t)
	t.Log("32000 shared-wrapper calls, 38400 remote attempts, five outcomes; all permits released")
}
func TestStressBudgetsAndAbnormalExit(t *testing.T) {
	if os.Getenv("RESILIENCE_STRESS") != "1" {
		t.Skip("set RESILIENCE_STRESS=1")
	}
	synctest.Test(t, func(t *testing.T) {
		for cycle := range 200 {
			l := limiter(t, 1)
			b := breaker(t, 1, func(error) bool { return false })
			cfg := config()
			cfg.Weight = 1
			cfg.AttemptTimeout = time.Millisecond
			cfg.TotalTimeout = time.Second
			marker := &domainError{cycle}
			f := wrap(t, func(ctx context.Context) (int, error) {
				switch cycle % 4 {
				case 0:
					<-ctx.Done()
					return cycle, ctx.Err()
				case 1:
					panic(marker)
				case 2:
					runtime.Goexit()
				}
				return cycle, nil
			}, cfg, resilience.Dependencies{Limiter: l, Breaker: b})
			done := make(chan struct{})
			go func() {
				defer close(done)
				work := func() {
					value, err := f(context.Background())
					if cycle%4 == 0 {
						if value != cycle || !errors.Is(err, resilience.ErrAttemptTimeout) {
							t.Error(value, err)
						}
					} else if cycle%4 == 3 && err != nil {
						t.Error(err)
					}
				}
				if cycle%4 == 1 {
					if catchPanic(work) != marker {
						t.Error("panic changed")
					}
				} else {
					work()
				}
			}()
			<-done
			acquireNow(t, l)()
			if b.Stats().InFlight != 0 {
				t.Fatal(b.Stats())
			}
		}
	})
	assertNoLeaks(t)
	t.Log("200 timeout/panic/Goexit/success cycles, permit and probe release")
}

type retainedPayload [64 << 10]byte
type payloadKey struct{}
type wrapperOwner struct{ fn client.Operation[int] }

func abandonedWrappers(t *testing.T) ([]weak.Pointer[retainedPayload], []weak.Pointer[wrapperOwner]) {
	t.Helper()
	var payloads []weak.Pointer[retainedPayload]
	var owners []weak.Pointer[wrapperOwner]
	for range 16 {
		payload := new(retainedPayload)
		payloads = append(payloads, weak.Make(payload))
		cfg := config()
		cfg.TotalTimeout = 2 * time.Hour
		cfg.AttemptTimeout = time.Hour
		cfg.AdmissionTimeout = time.Minute
		cfg.Weight = 1
		cfg.Tokens = 1
		l := limiter(t, 1)
		f := wrap(t, func(ctx context.Context) (int, error) { return int(ctx.Value(payloadKey{}).(*retainedPayload)[0]), nil }, cfg, resilience.Dependencies{Limiter: l, RateLimiter: rate(t, 1, 1), Classify: func(error) retry.Decision {
			if payload[0] == 0 {
				return retry.DecisionStop
			}
			return retry.DecisionReconcile
		}})
		owner := &wrapperOwner{f}
		owners = append(owners, weak.Make(owner))
		if _, err := owner.fn(context.WithValue(context.Background(), payloadKey{}, payload)); err != nil {
			t.Fatal(err)
		}
	}
	return payloads, owners
}
func TestStressReleasedClosures(t *testing.T) {
	if os.Getenv("RESILIENCE_STRESS") != "1" {
		t.Skip("set RESILIENCE_STRESS=1")
	}
	var baseline, peak uint64
	for cycle := range 30 {
		payloads, owners := abandonedWrappers(t)
		runtime.GC()
		runtime.GC()
		for _, p := range payloads {
			if p.Value() != nil {
				t.Fatal("wrapper/context retained payload")
			}
		}
		for _, owner := range owners {
			if owner.Value() != nil {
				t.Fatal("wrapper owner retained")
			}
		}
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		if cycle == 4 {
			baseline = mem.HeapAlloc
			peak = baseline
		}
		if cycle >= 5 {
			peak = max(peak, mem.HeapAlloc)
			if mem.HeapAlloc > baseline+(4<<20) {
				t.Fatal("post-GC fixture allowance exceeded", baseline, mem.HeapAlloc)
			}
		}
	}
	assertNoLeaks(t)
	t.Logf("480 abandoned wrappers/64KiB payloads, derived contexts/timers reclaimed; heap baseline=%d peak=%d", baseline, peak)
}
func FuzzConfig(f *testing.F) {
	f.Add(uint8(0), int64(0), int64(0), int64(0), int64(0))
	f.Add(uint8(31), int64(-1), int64(1), int64(1), int64(2))
	f.Fuzz(func(t *testing.T, mask uint8, total, admission, attempt, retries int64) {
		cfg := config()
		cfg.TotalTimeout = time.Duration(total)
		cfg.AdmissionTimeout = time.Duration(admission)
		cfg.AttemptTimeout = time.Duration(attempt)
		cfg.Retry.MaxRetries = int(retries)
		deps := resilience.Dependencies{Classify: retryAll}
		if mask&1 != 0 {
			cfg.Weight = 1
			deps.Limiter = limiter(t, 1)
		}
		if mask&2 != 0 {
			cfg.Tokens = 1
			deps.RateLimiter = rate(t, 1, 1)
		}
		if mask&4 != 0 {
			deps.Breaker = breaker(t, 5, func(error) bool { return true })
		}
		if mask&8 != 0 {
			cfg.Order = ""
		}
		if mask&16 != 0 {
			cfg.Weight = -1
		}
		valid := total >= 0 && admission >= 0 && attempt >= 0 && cfg.Retry.MaxRetries >= 0 && cfg.Weight >= 0 && (deps.Breaker == nil || cfg.Order != "")
		fn, err := resilience.Wrap(func(context.Context) (int, error) { t.Fatal("pre-canceled I/O"); return 0, nil }, cfg, deps)
		if (err == nil) != valid {
			t.Fatal(cfg, err)
		}
		if err != nil {
			if !errors.Is(err, resilience.ErrInvalidConfig) {
				t.Fatal(err)
			}
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := fn(ctx); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}
func FuzzSequences(f *testing.F) {
	f.Add([]byte{1, 1, 0}, uint8(7), uint8(2), uint8(5))
	f.Add([]byte{3}, uint8(0), uint8(0), uint8(1))
	f.Fuzz(func(t *testing.T, seq []byte, mask, retryByte, thresholdByte uint8) {
		if len(seq) == 0 || len(seq) > 32 {
			return
		}
		synctest.Test(t, func(t *testing.T) {
			cfg := config()
			cfg.Retry.MaxRetries = int(retryByte % 5)
			threshold := int(thresholdByte%5) + 1
			remote := &domainError{1}
			business := &domainError{2}
			uncertain := &domainError{3}
			unknown := &domainError{4}
			deps := resilience.Dependencies{}
			if mask&1 != 0 {
				cfg.Weight = 1
				deps.Limiter = limiter(t, 1)
			}
			if mask&2 != 0 {
				cfg.Tokens = 1
				deps.RateLimiter = rate(t, math.SmallestNonzeroFloat64, 16)
			}
			if mask&4 != 0 {
				deps.Breaker = breaker(t, uint64(threshold), func(err error) bool { return errors.Is(err, remote) })
			}
			classified, calls := 0, 0
			deps.Classify = func(err error) retry.Decision {
				classified++
				switch err {
				case remote:
					return retry.DecisionRetry
				case business:
					return retry.DecisionStop
				case uncertain:
					return retry.DecisionReconcile
				default:
					return "unknown"
				}
			}
			fn := wrap(t, func(context.Context) (int, error) {
				event := seq[calls%len(seq)] % 5
				calls++
				switch event {
				case 0:
					return calls + 100, nil
				case 1:
					return calls + 100, remote
				case 2:
					return calls + 100, business
				case 3:
					return calls + 100, uncertain
				default:
					return calls + 100, unknown
				}
			}, cfg, deps)
			expectedCalls, expectedClassified, attempts, failures := 0, 0, 0, 0
			expectedValue := 0
			expectedError := error(nil)
			signal := error(nil)
			open := false
			for repetition := 0; repetition <= cfg.Retry.MaxRetries; repetition++ {
				attempts++
				if open {
					expectedValue = 0
					expectedError = circuitbreaker.ErrOpen
					signal = resilience.ErrRejected
					break
				}
				event := seq[expectedCalls%len(seq)] % 5
				expectedCalls++
				expectedValue = 100 + expectedCalls
				if event == 0 {
					expectedError = nil
					if deps.Breaker != nil && failures+1 >= threshold && 2*failures >= failures+1 {
						open = true
					}
					break
				}
				expectedClassified++
				switch event {
				case 1:
					expectedError = remote
					failures++
					if deps.Breaker != nil && failures >= threshold {
						open = true
					}
				case 2:
					expectedError = business
				case 3:
					expectedError = uncertain
					signal = retry.ErrReconciliationRequired
				default:
					expectedError = unknown
					signal = retry.ErrInvalidDecision
				}
				if event != 1 {
					break
				}
			}
			value, err := fn(context.Background())
			if value != expectedValue || calls != expectedCalls || classified != expectedClassified {
				t.Fatal(seq, mask, value, err, calls, classified, expectedValue, expectedCalls, expectedClassified)
			}
			if signal != nil {
				if !errors.Is(err, signal) || !errors.Is(err, expectedError) {
					t.Fatal(err, signal, expectedError)
				}
			} else if err != expectedError {
				t.Fatal("identity", err, expectedError)
			}
			if deps.Limiter != nil {
				acquireNow(t, deps.Limiter)()
			}
			if deps.RateLimiter != nil {
				if !deps.RateLimiter.(*ratelimit.Limiter).Allow(16-attempts) || deps.RateLimiter.(*ratelimit.Limiter).Allow(1) {
					t.Fatal("per-attempt token charge")
				}
			}
			if deps.Breaker != nil && (deps.Breaker.(*circuitbreaker.Breaker).State() == circuitbreaker.StateOpen) != open {
				t.Fatal(deps.Breaker.(*circuitbreaker.Breaker).Stats())
			}
		})
	})
}
func FuzzBudgets(f *testing.F) {
	f.Add(uint8(0), uint8(5), uint8(3), uint8(2), uint8(4))
	f.Add(uint8(1), uint8(5), uint8(0), uint8(2), uint8(0))
	f.Fuzz(func(t *testing.T, stage, total, attempt, admission, parentByte uint8) {
		synctest.Test(t, func(t *testing.T) {
			cfg := config()
			cfg.TotalTimeout = time.Duration(total%6) * time.Millisecond
			cfg.AttemptTimeout = time.Duration(attempt%6) * time.Millisecond
			cfg.AdmissionTimeout = time.Duration(admission%6) * time.Millisecond
			parentDuration := time.Duration(parentByte%6+1) * time.Millisecond
			cause := errors.New("parent")
			ctx, cancel := context.WithTimeoutCause(context.Background(), parentDuration, cause)
			defer cancel()
			deps := resilience.Dependencies{}
			stage %= 3
			switch stage {
			case 1:
				cfg.Tokens = 1
				deps.RateLimiter = rate(t, math.SmallestNonzeroFloat64, 1)
				deps.RateLimiter.(*ratelimit.Limiter).Allow(1)
			case 2:
				cfg.Weight = 1
				deps.Limiter = limiter(t, 1)
				release := acquireNow(t, deps.Limiter)
				defer release()
			}
			minimum := parentDuration
			causes := map[error]time.Duration{cause: parentDuration}
			if cfg.TotalTimeout > 0 {
				causes[resilience.ErrTotalTimeout] = cfg.TotalTimeout
				minimum = min(minimum, cfg.TotalTimeout)
			}
			if stage == 0 && cfg.AttemptTimeout > 0 {
				causes[resilience.ErrAttemptTimeout] = cfg.AttemptTimeout
				minimum = min(minimum, cfg.AttemptTimeout)
			}
			if stage != 0 && cfg.AdmissionTimeout > 0 {
				causes[resilience.ErrAdmissionTimeout] = cfg.AdmissionTimeout
				minimum = min(minimum, cfg.AdmissionTimeout)
			}
			calls := 0
			fn := wrap(t, func(ctx context.Context) (int, error) { calls++; <-ctx.Done(); return 17, ctx.Err() }, cfg, deps)
			started := time.Now()
			value, err := fn(ctx)
			if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != minimum {
				t.Fatal(stage, cfg, err, time.Since(started), minimum)
			}
			found := false
			for marker, duration := range causes {
				if duration == minimum && errors.Is(err, marker) {
					found = true
				}
				if duration > minimum && errors.Is(err, marker) {
					t.Fatal("later budget misattributed", marker, err)
				}
			}
			if !found {
				t.Fatal("winning cause absent", stage, cfg, parentDuration, err)
			}
			if stage == 0 {
				if value != 17 || calls != 1 {
					t.Fatal(value, calls)
				}
			} else if value != 0 || calls != 0 || !errors.Is(err, resilience.ErrAdmission) {
				t.Fatal(value, err, calls)
			}
		})
	})
}
