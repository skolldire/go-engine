package concurrency

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"weak"
)

type retentionPayload [128 << 10]byte

func TestPoolReleasesCompletedPayloads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := poolConfig(2, 8)
		p, err := NewPool(cfg, func(_ context.Context, input *retentionPayload) (*retentionPayload, error) {
			output := new(retentionPayload)
			output[0] = input[0]
			return output, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { shutdownPool(t, p) })
		inputs, outputs := completedPayloads(t, p, 0)
		synctest.Wait()
		runtime.GC()
		runtime.GC()
		for i := range inputs {
			if inputs[i].Value() != nil || outputs[i].Value() != nil {
				t.Fatalf("completed payload %d retained by live pool", i)
			}
		}
		runtime.KeepAlive(p)
	})
}

func completedPayloads(t *testing.T, p *Pool[*retentionPayload, *retentionPayload], cycle int) ([]weak.Pointer[retentionPayload], []weak.Pointer[retentionPayload]) {
	t.Helper()
	var inputs, outputs []weak.Pointer[retentionPayload]
	for i := range 32 {
		payload := new(retentionPayload)
		payload[0] = byte(i)
		inputs = append(inputs, weak.Make(payload))
		future, err := p.Submit(testContext(t), Job[*retentionPayload]{ID: fmt.Sprintf("%d/%d", cycle, i), Weight: 1, Value: payload})
		if err != nil {
			t.Fatal(err)
		}
		result := outcome(t, future)
		if result.Err != nil || result.Value[0] != byte(i) {
			t.Fatalf("result: %+v", result)
		}
		outputs = append(outputs, weak.Make(result.Value))
	}
	return inputs, outputs
}

func TestStressResourcePlateau(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		if os.Getenv("CONCURRENCY_STRESS") != "1" {
			t.Skip("set CONCURRENCY_STRESS=1")
		}
		cfg := poolConfig(4, 32)
		p, err := NewPool(cfg, func(_ context.Context, input *retentionPayload) (*retentionPayload, error) {
			output := new(retentionPayload)
			output[0] = input[0]
			return output, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { shutdownPool(t, p) })
		var baseline, peak uint64
		for cycle := range 60 {
			inputs, outputs := completedPayloads(t, p, cycle)
			synctest.Wait()
			runtime.GC()
			for i := range inputs {
				if inputs[i].Value() != nil || outputs[i].Value() != nil {
					t.Fatalf("cycle %d retained payload %d", cycle, i)
				}
			}
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			if cycle == 9 {
				baseline = stats.HeapAlloc
				peak = baseline
			}
			if cycle >= 10 {
				if stats.HeapAlloc > peak {
					peak = stats.HeapAlloc
				}
				if stats.HeapAlloc > baseline+(8<<20) {
					t.Fatalf("heap failed to plateau: baseline=%d current=%d cycle=%d", baseline, stats.HeapAlloc, cycle)
				}
			}
			assertInvariant(t, p.Stats(), cfg)
		}
		shutdownPool(t, p)
		runtime.KeepAlive(p)
		assertNoGoroutineLeaks(t)
		t.Logf("60 cycles, 1920 input/output pairs of 128KiB, live-pool weak references cleared; post-GC heap baseline=%d peak=%d", baseline, peak)
	})
}

func TestRepeatedCanceledPipelineAndLimiterLifecycle(t *testing.T) {
	for cycle := range 30 {
		t.Run(fmt.Sprint(cycle), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				input, err := From(ctx, 1, 2, 3, 4, 5, 6, 7, 8)
				if err != nil {
					t.Fatal(err)
				}
				mapped, err := Map(ctx, input, StreamConfig{Workers: 4, Buffer: 2}, func(_ context.Context, n int) (int, error) { return n, nil })
				if err != nil {
					t.Fatal(err)
				}
				branches, err := FanOut(ctx, mapped, 3, 1)
				if err != nil {
					t.Fatal(err)
				}
				merged, err := FanIn(ctx, 1, branches...)
				if err != nil {
					t.Fatal(err)
				}
				batches, err := Batch(ctx, merged, BatchConfig{Size: 2, Buffer: 1, MaxBatchWeight: 2}, func(StreamResult[int]) int64 { return 1 })
				if err != nil {
					t.Fatal(err)
				}
				limiter, err := NewLimiter(LimiterConfig{Capacity: 4})
				if err != nil {
					t.Fatal(err)
				}
				release, err := limiter.Acquire(ctx, 4)
				if err != nil {
					t.Fatal(err)
				}
				var wg sync.WaitGroup
				var canceled atomic.Int32
				for range 16 {
					wg.Go(func() {
						r, e := limiter.Acquire(ctx, 1)
						if e != nil {
							canceled.Add(1)
						} else {
							r()
							t.Error("acquired saturated limiter")
						}
					})
				}
				synctest.Wait()
				cancel()
				wg.Wait()
				release()
				release()
				for range batches {
				}
				for range merged {
				}
				for _, branch := range branches {
					for range branch {
					}
				}
				for range mapped {
				}
				for range input {
				}
				if canceled.Load() != 16 {
					t.Fatal(canceled.Load())
				}
				r, e := limiter.Acquire(context.Background(), 4)
				if e != nil {
					t.Fatal(e)
				}
				r()
			})
		})
	}
	assertNoGoroutineLeaks(t)
}

func assertNoGoroutineLeaks(t *testing.T) {
	t.Helper()
	runtime.GC()
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		t.Fatal("goroutineleak profile missing")
	}
	var report bytes.Buffer
	if err := profile.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if profile.Count() != 0 {
		t.Fatalf("goroutine leaks: %s", report.String())
	}
	t.Logf("%s", report.String())
}

func TestPoolReleasesCanceledQueuedPayloads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		started := make(chan struct{}, 2)
		cfg := poolConfig(2, 8)
		p, err := NewPool(cfg, func(_ context.Context, input *retentionPayload) (int, error) {
			if input != nil {
				t.Error("canceled queued task executed")
				return 0, nil
			}
			started <- struct{}{}
			<-gate
			return 1, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { close(gate); shutdownPool(t, p) }()
		for i := range 2 {
			if _, err := p.Submit(context.Background(), Job[*retentionPayload]{ID: fmt.Sprint(i), Weight: 1}); err != nil {
				t.Fatal(err)
			}
		}
		for range 2 {
			<-started
		}
		pointers := cancelQueuedPayloads(t, p)
		synctest.Wait()
		runtime.GC()
		for i, pointer := range pointers {
			if pointer.Value() != nil {
				t.Fatalf("canceled payload %d retained", i)
			}
		}
		if p.Stats().Canceled != 8 {
			t.Fatal(p.Stats())
		}
		runtime.KeepAlive(p)
	})
}

func cancelQueuedPayloads(t *testing.T, p *Pool[*retentionPayload, int]) []weak.Pointer[retentionPayload] {
	t.Helper()
	var pointers []weak.Pointer[retentionPayload]
	for i := range 8 {
		payload := new(retentionPayload)
		pointers = append(pointers, weak.Make(payload))
		future, err := p.Submit(context.Background(), Job[*retentionPayload]{ID: fmt.Sprintf("queued%d", i), Weight: 1, Value: payload})
		if err != nil {
			t.Fatal(err)
		}
		future.Cancel(nil)
		if result := outcome(t, future); result.State != StateCanceled {
			t.Fatal(result)
		}
	}
	return pointers
}
