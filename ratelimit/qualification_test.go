package ratelimit

import (
	"bytes"
	"context"
	"errors"
	"math"
	"math/big"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
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
		t.Fatal("goroutineleak missing")
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
	l := newTestLimiter(t, Config{1, 1})
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		l.Wait(ctx, 1)
	}
	assertNoLeaks(t)
}
func TestStressConcurrentAdmissions(t *testing.T) {
	if os.Getenv("RATELIMIT_STRESS") != "1" {
		t.Skip("set RATELIMIT_STRESS=1")
	}
	synctest.Test(t, func(t *testing.T) {
		l := newTestLimiter(t, Config{1000, 128})
		var successes, weight atomic.Int64
		var wg sync.WaitGroup
		started := time.Now()
		for worker := range 32 {
			wg.Go(func() {
				for i := range 1000 {
					n := (i+worker)%3 + 1
					if i%3 == 0 {
						if l.Allow(n) {
							successes.Add(1)
							weight.Add(int64(n))
						}
					} else {
						ctx, cancel := context.WithCancel(context.Background())
						if i%7 == 0 {
							cancel()
						}
						err := l.Wait(ctx, n)
						cancel()
						if err == nil {
							successes.Add(1)
							weight.Add(int64(n))
						} else if !errors.Is(err, context.Canceled) {
							t.Error(err)
						}
					}
				}
			})
		}
		wg.Wait()
		pending(t, l, 0)
		elapsed := time.Since(started)
		bound := int64(128) + int64(elapsed)*1000/int64(time.Second)
		if weight.Load() > bound || timerOf(l) != nil {
			t.Fatal(weight.Load(), bound)
		}
		t.Logf("32000 mixed Wait/Allow/cancel calls; admitted calls=%d weight=%d elapsed=%v bound=%d", successes.Load(), weight.Load(), elapsed, bound)
	})
	assertNoLeaks(t)
}
func TestStressTimersAndHeadWakeups(t *testing.T) {
	if os.Getenv("RATELIMIT_STRESS") != "1" {
		t.Skip("set RATELIMIT_STRESS=1")
	}
	timers, notifications, canceled := 0, 0, 0
	synctest.Test(t, func(t *testing.T) {
		for range 20 {
			l := newTestLimiter(t, Config{1, 1})
			l.Allow(1)
			var done []<-chan error
			var cancel []context.CancelFunc
			for range 32 {
				ctx, stop := context.WithCancel(context.Background())
				cancel = append(cancel, stop)
				done = append(done, startWait(l, ctx, 1))
				synctest.Wait()
			}
			nodes := pending(t, l, 32)
			timer := timerOf(l)
			if timer == nil {
				t.Fatal("head timer missing")
			}
			timers++
			for i := 1; i < 32; i += 2 {
				cancel[i]()
				if !errors.Is(<-done[i], context.Canceled) {
					t.Fatal("middle cancellation")
				}
				canceled++
			}
			for index, w := range nodes {
				select {
				case <-w.ready:
					if index != 0 {
						t.Fatal("broadcast head notification")
					}
				default:
					if index == 0 {
						t.Fatal("missing head notification")
					}
				}
			}
			for i := 0; i < 32; i += 2 {
				pending(t, l, (32-i)/2)
				if timerOf(l) != timer {
					t.Fatal("more than one timer object in a live queue")
				}
				if i%4 == 0 {
					cancel[i]()
					if !errors.Is(<-done[i], context.Canceled) {
						t.Fatal("head cancellation")
					}
					canceled++
				} else {
					time.Sleep(time.Second)
					if err := <-done[i]; err != nil {
						t.Fatal(err)
					}
				}
				notifications++
			}
			pending(t, l, 0)
			if timerOf(l) != nil {
				t.Fatal("timer retained after drain")
			}
			for i, w := range nodes {
				select {
				case <-w.ready:
					if i%2 == 1 {
						t.Fatal("canceled nonhead notified")
					}
				default:
					if i%2 == 0 {
						t.Fatal("accepted head not notified")
					}
				}
				if w.prev != nil || w.next != nil {
					t.Fatal("detached node retained queue")
				}
			}
		}
	})
	assertNoLeaks(t)
	t.Logf("timers allocated across live queues=%d (one each); head notifications=%d; canceled waiters=%d; no nonhead broadcast", timers, notifications, canceled)
}

type retainedPayload [64 << 10]byte
type payloadKey struct{}

func abandonedWaits(t *testing.T) ([]weak.Pointer[retainedPayload], []weak.Pointer[waiter], weak.Pointer[time.Timer]) {
	t.Helper()
	l := newTestLimiter(t, Config{math.SmallestNonzeroFloat64, 1})
	l.Allow(1)
	var pointers []weak.Pointer[retainedPayload]
	var cancels []context.CancelFunc
	var done []<-chan error
	for range 16 {
		payload := new(retainedPayload)
		pointers = append(pointers, weak.Make(payload))
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), payloadKey{}, payload))
		cancels = append(cancels, cancel)
		done = append(done, startWait(l, ctx, 1))
		synctest.Wait()
	}
	nodes := pending(t, l, 16)
	references := make([]weak.Pointer[waiter], 0, len(nodes))
	for _, w := range nodes {
		references = append(references, weak.Make(w))
	}
	timer := weak.Make(timerOf(l))
	for _, cancel := range cancels {
		cancel()
	}
	for _, result := range done {
		if !errors.Is(<-result, context.Canceled) {
			t.Fatal("unexpected admission")
		}
	}
	pending(t, l, 0)
	return pointers, references, timer
}
func TestStressReleasedWaiters(t *testing.T) {
	if os.Getenv("RATELIMIT_STRESS") != "1" {
		t.Skip("set RATELIMIT_STRESS=1")
	}
	var baseline, peak uint64
	for cycle := range 30 {
		var payloads []weak.Pointer[retainedPayload]
		var nodes []weak.Pointer[waiter]
		var timer weak.Pointer[time.Timer]
		synctest.Test(t, func(t *testing.T) { payloads, nodes, timer = abandonedWaits(t) })
		runtime.GC()
		runtime.GC()
		for _, p := range payloads {
			if p.Value() != nil {
				t.Fatal("payload retained")
			}
		}
		for _, w := range nodes {
			if w.Value() != nil {
				t.Fatal("waiter retained")
			}
		}
		if timer.Value() != nil {
			t.Fatal("stopped timer retained")
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
				t.Fatal("post-GC heap exceeded fixture allowance", baseline, mem.HeapAlloc)
			}
		}
	}
	assertNoLeaks(t)
	t.Logf("480 canceled waits/64KiB payloads, 480 nodes and 30 long-delay timers reclaimed; heap baseline=%d peak=%d", baseline, peak)
}

func FuzzConfig(f *testing.F) {
	f.Add(math.Float64bits(1), int64(1), int64(1))
	f.Add(math.Float64bits(math.NaN()), int64(0), int64(-1))
	f.Fuzz(func(t *testing.T, bits uint64, burst, n int64) {
		cfg := Config{math.Float64frombits(bits), int(burst)}
		valid := cfg.RatePerSecond > 0 && !math.IsNaN(cfg.RatePerSecond) && !math.IsInf(cfg.RatePerSecond, 0) && cfg.Burst > 0
		l, err := New(cfg)
		if (err == nil) != valid {
			t.Fatal(cfg, err)
		}
		if err != nil {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatal(err)
			}
			return
		}
		weight := int(n)
		if weight <= 0 || weight > cfg.Burst {
			before := l.tokens
			if l.Allow(weight) || l.tokens != before {
				t.Fatal("invalid weight admitted")
			}
			if err := l.Wait(context.Background(), weight); !errors.Is(err, ErrInvalidCall) {
				t.Fatal(err)
			}
		}
	})
}
func exactCredit(rate float64, elapsed time.Duration) *big.Rat {
	return new(big.Rat).Mul(new(big.Rat).SetFloat64(rate), big.NewRat(int64(elapsed), int64(time.Second)))
}
func FuzzCredit(f *testing.F) {
	f.Add(math.Float64bits(3), uint64(7), uint64(333333334))
	f.Add(math.Float64bits(math.SmallestNonzeroFloat64), uint64(math.MaxInt), uint64(math.MaxInt64))
	f.Fuzz(func(t *testing.T, bits, capacity, elapsed uint64) {
		rate := math.Float64frombits(bits)
		burst := int(capacity & uint64(math.MaxInt))
		if !(rate > 0) || math.IsInf(rate, 0) || burst == 0 {
			return
		}
		l, err := New(Config{rate, burst})
		if err != nil {
			t.Fatal(err)
		}
		l.tokens = 0
		duration := time.Duration(elapsed & math.MaxInt64)
		start := l.anchor.at
		l.refill(start.Add(duration))
		credit := exactCredit(rate, duration)
		capRat := new(big.Rat).SetInt64(int64(burst))
		if credit.Cmp(capRat) > 0 {
			credit.Set(capRat)
		}
		whole := new(big.Int).Quo(credit.Num(), credit.Denom())
		if int64(l.tokens) != whole.Int64() {
			t.Fatal(rate, burst, duration, l.tokens, whole)
		}
		if l.tokens < burst {
			actual := new(big.Rat).Add(new(big.Rat).SetInt64(int64(l.tokens)), new(big.Rat).SetFrac(&l.anchor.phase, &l.denominator))
			if actual.Cmp(credit) != 0 {
				t.Fatal("lost phase", actual, credit)
			}
			delay := l.delay(burst)
			missing := new(big.Rat).Sub(capRat, credit)
			needed := new(big.Rat).Quo(missing, new(big.Rat).SetFloat64(rate))
			needed.Mul(needed, big.NewRat(int64(time.Second), 1))
			if needed.Cmp(big.NewRat(math.MaxInt64, 1)) > 0 {
				if delay != math.MaxInt64 {
					t.Fatal(delay)
				}
			} else {
				difference := new(big.Rat).Sub(big.NewRat(int64(delay), 1), needed)
				if difference.Sign() < 0 || difference.Cmp(big.NewRat(1, 1)) >= 0 {
					t.Fatal("early/excess delay", delay, needed)
				}
			}
		}
	})
}
func FuzzAccounting(f *testing.F) {
	f.Add(math.Float64bits(3), uint8(7), []byte{0, 10, 30, 1, 99, 25})
	f.Add(math.Float64bits(math.MaxFloat64), uint8(1), []byte{0, 0, 255, 0})
	f.Fuzz(func(t *testing.T, bits uint64, capByte uint8, seq []byte) {
		rate := math.Float64frombits(bits)
		if len(seq) > 64 || !(rate > 0) || math.IsInf(rate, 0) {
			return
		}
		burst := int(capByte%16) + 1
		l, err := New(Config{rate, burst})
		if err != nil {
			t.Fatal(err)
		}
		now := l.anchor.at
		credit := big.NewRat(int64(burst), 1)
		capRat := big.NewRat(int64(burst), 1)
		for _, event := range seq {
			if event%3 == 0 {
				n := int(event%byte(burst)) + 1
				before := new(big.Rat).Set(credit)
				enough := credit.Cmp(big.NewRat(int64(n), 1)) >= 0
				if enough {
					credit.Sub(credit, big.NewRat(int64(n), 1))
				}
				got := l.tokens >= n
				if got {
					l.tokens -= n
				}
				if got != enough {
					t.Fatal("admission diverged", before, n)
				}
			} else {
				delta := time.Duration(event) + 1
				now = now.Add(delta)
				l.refill(now)
				credit.Add(credit, exactCredit(rate, delta))
				if credit.Cmp(capRat) > 0 {
					credit.Set(capRat)
				}
			}
			actual := new(big.Rat).Add(big.NewRat(int64(l.tokens), 1), new(big.Rat).SetFrac(&l.anchor.phase, &l.denominator))
			if actual.Cmp(credit) != 0 || l.tokens < 0 || l.tokens > burst {
				t.Fatal("reference accounting diverged", actual, credit)
			}
		}
	})
}
func FuzzQueue(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5}, uint8(0))
	f.Add([]byte{9, 3, 1, 5}, uint8(7))
	f.Fuzz(func(t *testing.T, seq []byte, flags uint8) {
		if len(seq) > 16 {
			return
		}
		synctest.Test(t, func(t *testing.T) {
			l := newTestLimiter(t, Config{1, 4})
			l.Allow(4)
			var results []<-chan error
			var cancels []context.CancelFunc
			var weights []int
			expected := time.Duration(0)
			for _, event := range seq {
				n := int(event%4) + 1
				ctx, cancel := context.WithCancel(context.Background())
				cancels = append(cancels, cancel)
				results = append(results, startWait(l, ctx, n))
				weights = append(weights, n)
				synctest.Wait()
				if l.Allow(1) {
					t.Fatal("Allow bypass")
				}
			}
			for i, event := range seq {
				if event%3 == flags%3 {
					cancels[i]()
					if !errors.Is(<-results[i], context.Canceled) {
						t.Fatal("cancel consumption")
					}
					results[i] = nil
				}
			}
			pendingCount := 0
			for _, result := range results {
				if result != nil {
					pendingCount++
				}
			}
			pending(t, l, pendingCount)
			start := time.Now()
			for i, result := range results {
				if result == nil {
					continue
				}
				expected += time.Duration(weights[i]) * time.Second
				time.Sleep(time.Duration(weights[i]) * time.Second)
				if err := <-result; err != nil {
					t.Fatal(err)
				}
				if time.Since(start) != expected {
					t.Fatal("FIFO order/timing")
				}
				if l.Allow(1) {
					t.Fatal("unearned tokens")
				}
			}
			for _, cancel := range cancels {
				cancel()
			}
			pending(t, l, 0)
			if timerOf(l) != nil || l.tokens != 0 {
				t.Fatal("unfinished queue")
			}
		})
	})
}

func FuzzEnvelope(f *testing.F) {
	f.Add(uint8(2), uint8(3), []byte{0, 0, 0, 1, 2, 3, 4, 5, 6, 7})
	f.Fuzz(func(t *testing.T, rateIndex, capacity uint8, seq []byte) {
		if len(seq) > 32 {
			return
		}
		rates := []float64{math.SmallestNonzeroFloat64, 0.1, 3, 1e9, math.MaxFloat64}
		rate := rates[int(rateIndex)%len(rates)]
		burst := int(capacity%16) + 1
		synctest.Test(t, func(t *testing.T) {
			l := newTestLimiter(t, Config{rate, burst})
			type admission struct {
				at time.Time
				n  int
			}
			var history []admission
			for _, event := range seq {
				if event%2 == 1 {
					time.Sleep(time.Duration(event) * time.Nanosecond)
				} else {
					n := int(event)%burst + 1
					if l.Allow(n) {
						history = append(history, admission{time.Now(), n})
					}
				}
			}
			for start := range history {
				total := int64(0)
				for end := start; end < len(history); end++ {
					total += int64(history[end].n)
					credit := exactCredit(rate, history[end].at.Sub(history[start].at))
					limit := new(big.Int).Quo(credit.Num(), credit.Denom())
					limit.Add(limit, big.NewInt(int64(burst)))
					if limit.Cmp(big.NewInt(total)) < 0 {
						t.Fatal("interval over-admission", rate, burst, total, limit)
					}
				}
			}
		})
	})
}
func TestStressIndependentInstances(t *testing.T) {
	if os.Getenv("RATELIMIT_STRESS") != "1" {
		t.Skip("set RATELIMIT_STRESS=1")
	}
	synctest.Test(t, func(t *testing.T) {
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				l := newTestLimiter(t, Config{1000, 16})
				start := time.Now()
				total := int64(0)
				for i := range 500 {
					n := i%3 + 1
					if err := l.Wait(context.Background(), n); err != nil {
						t.Error(err)
						return
					}
					total += int64(n)
				}
				bound := int64(16) + int64(time.Since(start))*1000/int64(time.Second)
				if total > bound {
					t.Error(total, bound)
				}
			})
		}
		wg.Wait()
	})
	assertNoLeaks(t)
	t.Log("32 independent instances, 16000 weighted waits")
}
