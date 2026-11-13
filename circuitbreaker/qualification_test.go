package circuitbreaker

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
	"testing"
	"testing/synctest"
	"time"
	"weak"
)

func assertNoLeaks(t *testing.T) {
	t.Helper()
	runtime.GC()
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Fatal("goroutineleak profile missing")
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
	b := newTestBreaker(t, testConfig(), remote)
	for range 100 {
		Execute(context.Background(), b, func(context.Context) (int, error) { return 1, nil })
	}
	assertNoLeaks(t)
}
func TestStressConcurrentOutcomes(t *testing.T) {
	if os.Getenv("CIRCUITBREAKER_STRESS") != "1" {
		t.Skip("set CIRCUITBREAKER_STRESS=1")
	}
	cfg := testConfig()
	cfg.RequestThreshold = math.MaxUint64
	cfg.Interval = time.Hour
	original := errors.New("remote")
	business := errors.New("business")
	classifierPanic := errors.New("classifier panic")
	b := newTestBreaker(t, cfg, func(err error) bool {
		if err == classifierPanic {
			panic(err)
		}
		return err == original
	})
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for i := range 2000 {
				panicked := catchPanic(func() {
					got, err := Execute(context.Background(), b, func(context.Context) (int, error) {
						switch i % 5 {
						case 0:
							return 7, nil
						case 1:
							return 7, original
						case 2:
							return 7, business
						case 3:
							panic(original)
						default:
							return 7, classifierPanic
						}
					})
					if got != 7 || i%5 == 1 && err != original || i%5 == 2 && err != business {
						t.Error(got, err)
					}
				})
				if (i%5 >= 3) != (panicked != nil) {
					t.Error("abnormal termination lost")
				}
			}
		})
	}
	wg.Wait()
	c := b.Stats()
	checkCounts(t, c)
	if c != (Counts{State: StateClosed, Requests: 64000, Successes: 12800, Failures: 12800, Ignored: 12800, Panics: 25600}) {
		t.Fatal(c)
	}
	assertNoLeaks(t)
	t.Log("64000 concurrent executions, five outcomes including operation/classifier panic")
}

func TestStressGenerationChurn(t *testing.T) {
	if os.Getenv("CIRCUITBREAKER_STRESS") != "1" {
		t.Skip("set CIRCUITBREAKER_STRESS=1")
	}
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.HalfOpenMax = 2
		original := errors.New("remote")
		b := newTestBreaker(t, cfg, remote)
		for range 100 {
			var old sync.WaitGroup
			gate := make(chan struct{})
			for range 16 {
				old.Go(func() { Execute(context.Background(), b, func(context.Context) (int, error) { <-gate; return 7, nil }) })
			}
			synctest.Wait()
			trip(t, b)
			for range 100 {
				_, err := Execute(context.Background(), b, func(context.Context) (int, error) { t.Error("Open transport executed"); return 0, nil })
				if err != ErrOpen {
					t.Fatal(err)
				}
			}
			time.Sleep(cfg.OpenTimeout)
			failed := make(chan struct{})
			late := make(chan struct{})
			var probes sync.WaitGroup
			probes.Go(func() {
				Execute(context.Background(), b, func(context.Context) (int, error) { <-failed; return 3, original })
			})
			probes.Go(func() { Execute(context.Background(), b, func(context.Context) (int, error) { <-late; return 3, nil }) })
			synctest.Wait()
			var storm sync.WaitGroup
			for range 32 {
				storm.Go(func() {
					for range 20 {
						_, err := Execute(context.Background(), b, func(context.Context) (int, error) { t.Error("probe overflow"); return 0, nil })
						if err != ErrProbeLimit {
							t.Error(err)
						}
					}
				})
			}
			storm.Wait()
			close(failed)
			synctest.Wait()
			if b.State() != StateOpen {
				t.Fatal(b.Stats())
			}
			time.Sleep(cfg.OpenTimeout)
			for range 2 {
				Execute(context.Background(), b, func(context.Context) (int, error) { return 8, nil })
			}
			if b.State() != StateClosed {
				t.Fatal(b.Stats())
			}
			close(late)
			close(gate)
			probes.Wait()
			old.Wait()
			if b.Stats() != (Counts{State: StateClosed}) {
				t.Fatal("late completion changed new state", b.Stats())
			}
		}
	})
	assertNoLeaks(t)
	t.Log("100 recovery cycles, 10000 open and 64000 probe-limit rejections, 1700 late completions")
}

type retainedPayload [64 << 10]byte

func abandonedInstances(t *testing.T) []weak.Pointer[retainedPayload] {
	t.Helper()
	pointers := make([]weak.Pointer[retainedPayload], 0, 16)
	for i := range 16 {
		payload := new(retainedPayload)
		payload[0] = byte(i)
		pointers = append(pointers, weak.Make(payload))
		cfg := testConfig()
		b := newTestBreaker(t, cfg, func(error) bool { return payload[0] > 128 })
		Execute(context.Background(), b, func(context.Context) (int, error) { return 1, errors.New("business") })
	}
	return pointers
}
func retiredGenerations(b *Breaker) []weak.Pointer[generation] {
	pointers := make([]weak.Pointer[generation], 0, 100)
	now := b.current.expires
	for i := range 100 {
		pointers = append(pointers, weak.Make(b.current))
		b.advance(now.Add(time.Duration(i+1) * b.cfg.Interval))
	}
	return pointers
}
func TestStressReleasedState(t *testing.T) {
	if os.Getenv("CIRCUITBREAKER_STRESS") != "1" {
		t.Skip("set CIRCUITBREAKER_STRESS=1")
	}
	var baseline, peak uint64
	b := newTestBreaker(t, testConfig(), remote)
	for cycle := range 60 {
		pointers := abandonedInstances(t)
		retired := retiredGenerations(b)
		runtime.GC()
		runtime.GC()
		for _, p := range pointers {
			if p.Value() != nil {
				t.Fatal("abandoned breaker retained classifier payload")
			}
		}
		for _, g := range retired {
			if g.Value() != nil {
				t.Fatal("retired generation retained")
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
				t.Fatal("heap exceeded fixture allowance", baseline, mem.HeapAlloc)
			}
		}
	}
	runtime.KeepAlive(b)
	assertNoLeaks(t)
	t.Logf("960 abandoned instances/64KiB classifier payloads, 6000 retired generations; heap baseline=%d peak=%d", baseline, peak)
}

func FuzzConfig(f *testing.F) {
	f.Add("service", uint64(5), math.Float64bits(0.5), int64(time.Minute), int64(time.Second), uint64(1))
	f.Add("", uint64(0), math.Float64bits(math.NaN()), int64(0), int64(-1), uint64(0))
	f.Fuzz(func(t *testing.T, name string, threshold, bits uint64, interval, timeout int64, probes uint64) {
		if len(name) > 4096 {
			return
		}
		cfg := Config{name, threshold, math.Float64frombits(bits), time.Duration(interval), time.Duration(timeout), probes}
		err := cfg.Validate()
		valid := name != "" && threshold > 0 && cfg.FailureRate > 0 && cfg.FailureRate <= 1 && !math.IsNaN(cfg.FailureRate) && !math.IsInf(cfg.FailureRate, 0) && interval > 0 && timeout > 0 && probes > 0
		if (err == nil) != valid {
			t.Fatal(cfg, err)
		}
		if err != nil && !errors.Is(err, ErrInvalidConfig) {
			t.Fatal(err)
		}
	})
}
func FuzzRate(f *testing.F) {
	f.Add(uint64(math.MaxUint64), uint64(math.MaxUint64-1), math.Float64bits(1))
	f.Add(uint64(10), uint64(1), math.Float64bits(0.1))
	f.Fuzz(func(t *testing.T, completed, failures, bits uint64) {
		rate := math.Float64frombits(bits)
		if completed == 0 || failures > completed || !(rate > 0 && rate <= 1) {
			return
		}
		ratio := new(big.Rat).SetFrac(new(big.Int).SetUint64(failures), new(big.Int).SetUint64(completed))
		want := ratio.Cmp(new(big.Rat).SetFloat64(rate)) >= 0
		if reachedRate(failures, completed, rate) != want {
			t.Fatal(completed, failures, rate)
		}
	})
}

type reference struct {
	state                                                    State
	due, now                                                 int
	epoch                                                    int
	requests, inflight, successes, failures, ignored, panics uint64
}

func (m *reference) enter(state State) {
	m.state = state
	m.epoch++
	m.requests = 0
	m.inflight = 0
	m.successes = 0
	m.failures = 0
	m.ignored = 0
	m.panics = 0
	m.due = 0
	if state == StateClosed {
		m.due = m.now + 3
	}
	if state == StateOpen {
		m.due = m.now + 2
	}
}
func (m *reference) observe() {
	if m.due != 0 && m.now >= m.due {
		if m.state == StateClosed {
			m.enter(StateClosed)
		} else {
			m.enter(StateHalfOpen)
		}
	}
}
func (m *reference) reserve() (int, error) {
	m.observe()
	if m.state == StateOpen {
		return 0, ErrOpen
	}
	if m.state == StateHalfOpen && m.inflight == 2 {
		return 0, ErrProbeLimit
	}
	m.requests++
	m.inflight++
	return m.epoch, nil
}
func (m *reference) complete(epoch int, result byte) {
	m.observe()
	if epoch != m.epoch {
		return
	}
	m.inflight--
	switch result % 4 {
	case 0:
		m.successes++
	case 1:
		m.failures++
	case 2:
		m.ignored++
	case 3:
		m.panics++
	}
	if m.state == StateHalfOpen {
		if result%4 == 1 {
			m.enter(StateOpen)
		} else if m.successes >= 2 {
			m.enter(StateClosed)
		}
	} else if m.state == StateClosed && m.successes+m.failures >= 3 && m.failures*2 >= m.successes+m.failures {
		m.enter(StateOpen)
	}
}
func (m *reference) counts() Counts {
	m.observe()
	return Counts{m.state, m.requests, m.inflight, m.successes, m.failures, m.ignored, m.panics}
}
func FuzzTransitions(f *testing.F) {
	f.Add([]byte{0, 0, 0, 1, 1, 1, 7, 7, 0, 0, 1, 1})
	f.Add([]byte{0, 5, 0, 2, 0, 3, 0, 4, 6, 7})
	f.Fuzz(func(t *testing.T, seq []byte) {
		if len(seq) > 128 {
			return
		}
		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			cfg.RequestThreshold = 3
			cfg.HalfOpenMax = 2
			cfg.Interval = 3 * time.Second
			cfg.OpenTimeout = 2 * time.Second
			b := newTestBreaker(t, cfg, remote)
			m := reference{}
			m.enter(StateClosed)
			type token struct {
				g     *generation
				epoch int
			}
			var pending []token
			for _, event := range seq {
				switch event % 8 {
				case 0:
					if len(pending) < 32 {
						g, err := b.admit(context.Background())
						epoch, want := m.reserve()
						if err != want {
							t.Fatal(err, want)
						}
						if err == nil {
							pending = append(pending, token{g, epoch})
						}
					}
				case 1, 2, 3, 4:
					if len(pending) > 0 {
						index := int(event/8) % len(pending)
						p := pending[index]
						pending = append(pending[:index], pending[index+1:]...)
						result := (event % 8) - 1
						b.finish(p.g, outcome(result))
						m.complete(p.epoch, result)
					}
				case 5, 6, 7:
					delta := int(event%3) + 1
					time.Sleep(time.Duration(delta) * time.Second)
					m.now += delta
				}
				got := b.Stats()
				want := m.counts()
				if got != want {
					t.Fatal(seq, event, got, want)
				}
				checkCounts(t, got)
			}
			for _, p := range pending {
				b.finish(p.g, ignored)
				m.complete(p.epoch, 2)
			}
			if b.Stats() != m.counts() {
				t.Fatal(b.Stats(), m.counts())
			}
		})
	})
}
func FuzzRollover(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint64(0))
	f.Add(uint8(1), uint8(3), uint64(10))
	f.Fuzz(func(t *testing.T, stateIndex, resultIndex uint8, remaining uint64) {
		cfg := testConfig()
		cfg.RequestThreshold = math.MaxUint64
		cfg.HalfOpenMax = math.MaxUint64
		cfg.Interval = time.Hour
		b := newTestBreaker(t, cfg, remote)
		state := StateClosed
		if stateIndex%2 == 1 {
			state = StateHalfOpen
		}
		b.start(state, time.Now())
		g := b.current
		g.counts.Requests = math.MaxUint64
		g.counts.Successes = math.MaxUint64 - (remaining % 16)
		g.counts.InFlight = remaining % 16
		next, err := b.admit(context.Background())
		if err != nil || next == g {
			t.Fatal(err)
		}
		b.finish(next, outcome(resultIndex%4))
		checkCounts(t, b.Stats())
		before := b.Stats()
		if g.counts.InFlight > 0 {
			b.finish(g, success)
			if b.Stats() != before {
				t.Fatal("retired overflow generation changed state")
			}
		}
	})
}
