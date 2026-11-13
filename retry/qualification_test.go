package retry

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"testing/synctest"
	"time"
	"weak"
)

func assertNoLeaks(t *testing.T) {
	t.Helper()
	runtime.GC()
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		t.Fatal("missing goroutineleak profile")
	}
	var report bytes.Buffer
	if err := profile.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if profile.Count() != 0 {
		t.Fatal(report.String())
	}
	t.Log(report.String())
}
func TestNoGoroutineLeaks(t *testing.T) {
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		Do(ctx, DefaultConfig(), nil, func(context.Context) (int, error) { return 0, nil })
	}
	assertNoLeaks(t)
}
func TestStressConcurrent(t *testing.T) {
	if os.Getenv("RETRY_STRESS") != "1" {
		t.Skip("set RETRY_STRESS=1")
	}
	var wg sync.WaitGroup
	for worker := range 32 {
		wg.Go(func() {
			for i := range 1000 {
				cfg := fastConfig()
				cfg.MaxRetries = 3
				if i%2 == 0 {
					cfg.Jitter = JitterFull
				}
				ctx, cancel := context.WithCancelCause(context.Background())
				cause := errors.New("cancel")
				original := &domainError{worker}
				calls := 0
				decision := DecisionRetry
				switch i % 4 {
				case 1:
					decision = DecisionReconcile
				case 2:
					decision = DecisionStop
				}
				got, err := Do(ctx, cfg, func(error) Decision { return decision }, func(context.Context) (int, error) {
					calls++
					if i%4 == 3 {
						cancel(cause)
					}
					if i%4 == 0 && calls == 3 {
						return worker, nil
					}
					return worker, original
				})
				cancel(nil)
				if got != worker {
					t.Error(got)
				}
				switch i % 4 {
				case 0:
					if err != nil || calls != 3 {
						t.Error(err, calls)
					}
				case 1:
					if !errors.Is(err, ErrReconciliationRequired) || calls != 1 {
						t.Error(err, calls)
					}
				case 2:
					if err != original || calls != 1 {
						t.Error(err, calls)
					}
				case 3:
					if !errors.Is(err, cause) || !errors.Is(err, original) || calls != 1 {
						t.Error(err, calls)
					}
				}
			}
		})
	}
	wg.Wait()
	assertNoLeaks(t)
	t.Log("32000 calls: retry success, reconcile, stop, cancellation; concurrent full jitter")
}

type retainedPayload [64 << 10]byte
type payloadKey struct{}

func releaseContexts(t *testing.T) []weak.Pointer[retainedPayload] {
	t.Helper()
	pointers := make([]weak.Pointer[retainedPayload], 0, 16)
	for range 16 {
		payload := new(retainedPayload)
		pointers = append(pointers, weak.Make(payload))
		ctx := context.WithValue(context.Background(), payloadKey{}, payload)
		cfg := DefaultConfig()
		cfg.TotalTimeout = time.Hour
		_, err := Do(ctx, cfg, nil, func(ctx context.Context) (int, error) { return int(ctx.Value(payloadKey{}).(*retainedPayload)[0]), nil })
		if err != nil {
			t.Fatal(err)
		}
	}
	return pointers
}
func TestStressReleasedContexts(t *testing.T) {
	if os.Getenv("RETRY_STRESS") != "1" {
		t.Skip("set RETRY_STRESS=1")
	}
	var baseline, peak uint64
	for cycle := range 60 {
		pointers := releaseContexts(t)
		runtime.GC()
		runtime.GC()
		for _, p := range pointers {
			if p.Value() != nil {
				t.Fatal("timer/context retained payload")
			}
		}
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		if cycle == 9 {
			baseline = mem.HeapAlloc
			peak = baseline
		}
		if cycle >= 10 {
			peak = max(peak, mem.HeapAlloc)
			if mem.HeapAlloc > baseline+(4<<20) {
				t.Fatal("post-GC heap exceeded fixture allowance", baseline, mem.HeapAlloc)
			}
		}
	}
	assertNoLeaks(t)
	t.Logf("960 released hour-deadline contexts with 64KiB payloads; heap baseline=%d peak=%d", baseline, peak)
}

func FuzzConfig(f *testing.F) {
	f.Add(int64(0), int64(1), int64(10), int64(0), math.Float64bits(2), "full")
	f.Add(int64(-1), int64(0), int64(0), int64(-1), math.Float64bits(math.NaN()), "invalid")
	f.Fuzz(func(t *testing.T, retries, initial, cap, timeout int64, bits uint64, jitter string) {
		cfg := Config{int(retries), time.Duration(initial), time.Duration(cap), time.Duration(timeout), math.Float64frombits(bits), jitter}
		before := cfg
		err := cfg.Validate()
		valid := cfg.MaxRetries >= 0 && initial > 0 && cap >= initial && timeout >= 0 && cfg.Multiplier >= 1 && !math.IsNaN(cfg.Multiplier) && !math.IsInf(cfg.Multiplier, 0) && (jitter == JitterFull || jitter == JitterNone)
		if (err == nil) != valid || math.Float64bits(cfg.Multiplier) != math.Float64bits(before.Multiplier) || cfg.MaxRetries != before.MaxRetries {
			t.Fatal(cfg, err)
		}
		if err != nil && !errors.Is(err, ErrInvalidConfig) {
			t.Fatal(err)
		}
	})
}
func FuzzBackoff(f *testing.F) {
	f.Add(uint64(1), uint64(math.MaxInt64), math.Float64bits(2), uint64(63))
	f.Add(uint64(math.MaxInt64), uint64(math.MaxInt64), math.Float64bits(1), uint64(math.MaxInt))
	f.Fuzz(func(t *testing.T, initial, cap, bits, k uint64) {
		cfg := DefaultConfig()
		cfg.InitialBackoff = time.Duration(initial & math.MaxInt64)
		cfg.MaxBackoff = time.Duration(cap & math.MaxInt64)
		cfg.Multiplier = math.Float64frombits(bits)
		if cfg.Validate() != nil {
			return
		}
		index := int(k & uint64(math.MaxInt))
		got := backoff(cfg, index)
		if got < cfg.InitialBackoff || got > cfg.MaxBackoff {
			t.Fatal(cfg, index, got)
		}
		if index > 0 && got < backoff(cfg, index-1) {
			t.Fatal("nonmonotonic")
		}
		for _, draw := range []func(uint64) uint64{func(uint64) uint64 { return 0 }, func(n uint64) uint64 { return n - 1 }} {
			j := fullJitter(got, draw)
			if j < 0 || j > got {
				t.Fatal(j, got)
			}
		}
	})
}
func FuzzDecisions(f *testing.F) {
	f.Add([]byte{0, 0, 1}, uint8(2), false)
	f.Add([]byte{2}, uint8(0), false)
	f.Fuzz(func(t *testing.T, seq []byte, retries uint8, cancelFirst bool) {
		if len(seq) == 0 || len(seq) > 64 {
			return
		}
		cfg := fastConfig()
		cfg.MaxRetries = int(retries % 17)
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		cause := errors.New("cause")
		if cancelFirst {
			cancel(cause)
		}
		original := &domainError{1}
		calls, classified := 0, 0
		last := DecisionRetry
		success := false
		got, err := Do(ctx, cfg, func(error) Decision { classified++; return last }, func(context.Context) (int, error) {
			b := seq[calls%len(seq)] % 5
			calls++
			switch b {
			case 0:
				last = DecisionRetry
			case 1:
				last = DecisionStop
			case 2:
				last = DecisionReconcile
			case 3:
				last = "unknown"
			case 4:
				success = true
				return calls, nil
			}
			return calls, original
		})
		if cancelFirst {
			if calls != 0 || got != 0 || !errors.Is(err, cause) {
				t.Fatal(got, err, calls)
			}
			return
		}
		if got != calls || calls > cfg.MaxRetries+1 || classified != calls-boolInt(success) {
			t.Fatal(got, err, calls, classified)
		}
		switch {
		case success:
			if err != nil {
				t.Fatal(err)
			}
		case last == DecisionStop || last == DecisionRetry:
			if err != original {
				t.Fatal(err)
			}
		case last == DecisionReconcile:
			if !errors.Is(err, ErrReconciliationRequired) || !errors.Is(err, original) {
				t.Fatal(err)
			}
		default:
			if !errors.Is(err, ErrInvalidDecision) || !errors.Is(err, original) {
				t.Fatal(err)
			}
		}
	})
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func canceledWaits(t *testing.T) []weak.Pointer[retainedPayload] {
	t.Helper()
	var wg sync.WaitGroup
	cancels := make([]context.CancelFunc, 64)
	pointers := make([]weak.Pointer[retainedPayload], 64)
	for i := range 64 {
		payload := new(retainedPayload)
		pointers[i] = weak.Make(payload)
		parent := context.WithValue(context.Background(), payloadKey{}, payload)
		ctx, cancel := context.WithCancel(parent)
		cancels[i] = cancel
		wg.Go(func() {
			cfg := DefaultConfig()
			cfg.MaxRetries = 3
			cfg.InitialBackoff = time.Hour
			cfg.MaxBackoff = time.Hour
			cfg.TotalTimeout = 2 * time.Hour
			cfg.Jitter = JitterNone
			calls := 0
			original := errors.New("transient")
			_, err := Do(ctx, cfg, retryAll, func(ctx context.Context) (int, error) {
				calls++
				return int(ctx.Value(payloadKey{}).(*retainedPayload)[0]), original
			})
			if calls != 1 || !errors.Is(err, context.Canceled) || !errors.Is(err, original) {
				t.Error(calls, err)
			}
		})
	}
	synctest.Wait()
	for _, cancel := range cancels {
		cancel()
	}
	wg.Wait()
	synctest.Wait()
	return pointers
}
func TestStressCanceledWaitTimers(t *testing.T) {
	if os.Getenv("RETRY_STRESS") != "1" {
		t.Skip("set RETRY_STRESS=1")
	}
	var pointers []weak.Pointer[retainedPayload]
	synctest.Test(t, func(t *testing.T) {
		for range 20 {
			pointers = append(pointers, canceledWaits(t)...)
		}
	})
	runtime.GC()
	runtime.GC()
	for _, pointer := range pointers {
		if pointer.Value() != nil {
			t.Fatal("canceled wait retained payload")
		}
	}
	assertNoLeaks(t)
	t.Log("1280 blocked hour-long waits canceled; timers, contexts and 64KiB payloads released after synctest bubble exit")
}
