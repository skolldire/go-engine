package concurrency

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestPoolConfigurationMatrix(t *testing.T) {
	for _, workers := range []int{1, 2, 8} {
		for _, queue := range []int{0, 1, 16} {
			for _, admission := range []Admission{AdmissionBlock, AdmissionReject} {
				for _, keyed := range []bool{false, true} {
					for _, weighted := range []bool{false, true} {
						name := fmt.Sprintf("w%d/q%d/%s/keyed=%t/weighted=%t", workers, queue, admission, keyed, weighted)
						t.Run(name, func(t *testing.T) {
							cfg := DefaultPoolConfig()
							cfg.Workers, cfg.QueueCapacity, cfg.Admission = workers, queue, admission
							cfg.MaxInFlightWeight = int64(workers + queue)
							if weighted {
								cfg.MaxJobWeight = 3
								cfg.MaxInFlightWeight *= 3
							}
							if keyed {
								cfg.Ordering = OrderingPerKey
								cfg.MaxActiveKeys = 3
								cfg.PerKeyCapacity = 2
								cfg.KeyFailure = KeyFailureContinue
							}
							exercisePoolConfiguration(t, cfg)
						})
					}
				}
			}
		}
	}
}

func exercisePoolConfiguration(t *testing.T, cfg PoolConfig) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var mu sync.Mutex
	active := map[int]bool{}
	starts := map[int][]int{}
	violations := make(chan error, 1)
	p, err := NewPool(cfg, func(_ context.Context, n int) (int, error) {
		key := n % 3
		mu.Lock()
		if cfg.Ordering == OrderingPerKey && active[key] {
			select {
			case violations <- fmt.Errorf("overlapping key %d", key):
			default:
			}
		}
		active[key] = true
		starts[key] = append(starts[key], n)
		mu.Unlock()
		runtime.Gosched()
		mu.Lock()
		active[key] = false
		mu.Unlock()
		return n * 2, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Shutdown(ctx, ShutdownCancel); err != nil {
			t.Error(err)
		}
	})
	type accepted struct {
		value  int
		future *Future[int]
	}
	results := make(chan accepted, 256)
	failures := make(chan error, 256)
	var producers sync.WaitGroup
	for producer := range 8 {
		producers.Go(func() {
			for index := range 32 {
				n := producer*32 + index
				key := ""
				if cfg.Ordering == OrderingPerKey {
					key = fmt.Sprint(n % 3)
				}
				weight := int64(1) + int64(n)%cfg.MaxJobWeight
				future, err := p.Submit(ctx, Job[int]{ID: fmt.Sprint(n), Key: key, Weight: weight, Value: n})
				if err != nil {
					if cfg.Admission != AdmissionReject || !errors.Is(err, ErrSaturated) {
						failures <- err
					}
					continue
				}
				results <- accepted{n, future}
			}
		})
	}
	done := make(chan struct{})
	go func() { producers.Wait(); close(done) }()
	observing := true
	for observing {
		assertInvariant(t, p.Stats(), cfg)
		select {
		case <-done:
			observing = false
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
			runtime.Gosched()
		}
	}
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if err := p.Shutdown(ctx, ShutdownDrain); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-violations:
		t.Error(err)
	default:
	}
	perKey := map[int][]Result[int]{}
	seen := map[uint64]bool{}
	for accepted := range results {
		result, err := accepted.future.Wait(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if result.State != StateSucceeded || result.Err != nil || result.Value != accepted.value*2 || result.ID != fmt.Sprint(accepted.value) {
			t.Fatalf("result: %+v", result)
		}
		if seen[result.Sequence] {
			t.Fatalf("duplicate sequence %d", result.Sequence)
		}
		seen[result.Sequence] = true
		perKey[accepted.value%3] = append(perKey[accepted.value%3], result)
	}
	if cfg.Ordering == OrderingPerKey {
		for key, results := range perKey {
			sort.Slice(results, func(i, j int) bool { return results[i].Sequence < results[j].Sequence })
			for i, result := range results {
				if starts[key][i] != result.Value/2 {
					t.Fatalf("key %d acceptance order differs from execution", key)
				}
			}
		}
	}
	stats := p.Stats()
	assertInvariant(t, stats, cfg)
	if stats.Accepted != uint64(len(seen)) || stats.Succeeded != stats.Accepted || stats.ActiveKeys != 0 || stats.InFlightWeight != 0 || stats.Queued != 0 || stats.Running != 0 {
		t.Fatalf("terminal: %+v", stats)
	}
	if cfg.Admission == AdmissionBlock && stats.Accepted != 256 {
		t.Fatalf("lost submissions: %+v", stats)
	}
}

func TestStressPoolLifecycle(t *testing.T) {
	if os.Getenv("CONCURRENCY_STRESS") != "1" {
		t.Skip("set CONCURRENCY_STRESS=1")
	}
	for cycle := range 100 {
		for _, mode := range []ShutdownMode{ShutdownDrain, ShutdownCancel} {
			t.Run(fmt.Sprintf("cycle%d/%s", cycle, mode), func(t *testing.T) { stressPoolLifecycle(t, mode) })
		}
	}
}

func stressPoolLifecycle(t *testing.T, mode ShutdownMode) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := poolConfig(4, 32)
	cfg.Ordering, cfg.MaxActiveKeys, cfg.PerKeyCapacity, cfg.KeyFailure = OrderingPerKey, 8, 8, KeyFailureCancelPending
	gate := make(chan struct{})
	taskStarted := make(chan struct{}, 4)
	p, err := NewPool(cfg, func(ctx context.Context, n int) (int, error) {
		if n < 0 {
			taskStarted <- struct{}{}
			<-gate
			return n, nil
		}
		switch n % 11 {
		case 0:
			return 0, errors.New("task failure")
		case 1:
			panic("task panic")
		case 2:
			<-ctx.Done()
			return 0, ctx.Err()
		default:
			runtime.Gosched()
			return n, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	var submitters sync.WaitGroup
	futures := make(chan *Future[int], 256)
	failures := make(chan error, 256)
	cancellations := make(chan context.CancelFunc, 256)
	started := make(chan struct{}, 8)
	for i := range 4 {
		future, err := p.Submit(ctx, Job[int]{ID: fmt.Sprintf("seed%d", i), Key: fmt.Sprint(i), Weight: 1, Value: -1 - i})
		if err != nil {
			t.Fatal(err)
		}
		futures <- future
	}
	for range 4 {
		<-taskStarted
	}
	for producer := range 8 {
		submitters.Go(func() {
			started <- struct{}{}
			for i := range 32 {
				n := producer*32 + i
				jobCtx, jobCancel := context.WithCancel(ctx)
				if n%7 == 0 {
					jobCancel()
				}
				future, err := p.Submit(jobCtx, Job[int]{ID: fmt.Sprint(n), Key: fmt.Sprint(n % 8), Weight: int64(1 + n%4), Value: n})
				if err == nil {
					futures <- future
					if n%3 == 0 {
						future.Cancel(nil)
					}
				} else if !errors.Is(err, ErrClosed) && !errors.Is(err, context.Canceled) {
					failures <- err
				}
				if err != nil || n%3 == 0 || n%11 == 2 {
					jobCancel()
				} else {
					cancellations <- jobCancel
				}
			}
		})
	}
	for range 8 {
		<-started
	}
	var accepted []*Future[int]
	for range 16 {
		select {
		case future := <-futures:
			accepted = append(accepted, future)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		assertInvariant(t, p.Stats(), cfg)
	}
	shutdownErrors := make(chan error, 4)
	for range 4 {
		go func() { shutdownErrors <- p.Shutdown(ctx, mode) }()
	}
	close(gate)
	submitters.Wait()
	close(futures)
	close(failures)
	close(cancellations)
	defer func() {
		for cancel := range cancellations {
			cancel()
		}
	}()
	for range 4 {
		if err := <-shutdownErrors; err != nil {
			t.Fatal(err)
		}
	}
	for err := range failures {
		t.Error(err)
	}
	counts := map[State]uint64{}
	for future := range futures {
		accepted = append(accepted, future)
	}
	for _, future := range accepted {
		result, err := future.Wait(ctx)
		if err != nil {
			t.Fatal(err)
		}
		again, err := future.Wait(ctx)
		if err != nil || again.Sequence != result.Sequence || again.State != result.State {
			t.Fatal("unstable terminal result")
		}
		counts[result.State]++
	}
	stats := p.Stats()
	assertInvariant(t, stats, cfg)
	if stats.Succeeded != counts[StateSucceeded] || stats.Failed != counts[StateFailed] || stats.Canceled != counts[StateCanceled] || stats.ActiveKeys != 0 || stats.InFlightWeight != 0 || stats.Queued != 0 || stats.Running != 0 {
		t.Fatalf("terminal accounting: %+v vs %v", stats, counts)
	}
	select {
	case <-p.Done():
	default:
		t.Fatal("shutdown returned before Done")
	}
}

func TestStressSustainedWorkload(t *testing.T) {
	if os.Getenv("CONCURRENCY_STRESS") != "1" {
		t.Skip("set CONCURRENCY_STRESS=1")
	}
	end := time.Now().Add(15 * time.Second)
	cycles := 0
	for time.Now().Before(end) {
		t.Run(fmt.Sprint(cycles), func(t *testing.T) {
			cfg := poolConfig(4, 16)
			cfg.Admission = AdmissionReject
			if cycles%2 == 0 {
				cfg.Ordering = OrderingPerKey
				cfg.MaxActiveKeys = 3
				cfg.PerKeyCapacity = 4
				cfg.KeyFailure = KeyFailureContinue
			}
			exercisePoolConfiguration(t, cfg)
		})
		cycles++
	}
	assertNoGoroutineLeaks(t)
	t.Logf("sustained 15s workload: %d pool lifecycles, %d submission attempts", cycles, cycles*256)
}
