package concurrency

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func FuzzPoolConfig(f *testing.F) {
	f.Add(1, 0, int64(1), "block", "unordered")
	f.Add(-1, -2, int64(-3), "bad", "per_key")
	f.Fuzz(func(t *testing.T, workers, queue int, weight int64, admission, ordering string) {
		c := DefaultPoolConfig()
		c.Workers = workers
		c.QueueCapacity = queue
		c.MaxJobWeight = weight
		c.Admission = Admission(admission)
		c.Ordering = Ordering(ordering)
		copy := c
		err := c.Validate()
		valid := workers > 0 && queue >= 0 && weight > 0 && weight <= c.MaxInFlightWeight && (admission == "block" || admission == "reject") && ordering == "unordered"
		if (err == nil) != valid {
			t.Fatalf("validation disagrees with constraints: %+v, %v", c, err)
		}
		if err != nil && !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("unclassified config error: %v", err)
		}
		if c != copy {
			t.Fatal("Validate mutated Config")
		}
	})
}
func FuzzPoolTransitions(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5})
	f.Add([]byte{3, 3, 1, 4, 0, 2})
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 64 {
			ops = ops[:64]
		}
		cfg := poolConfig(2, 4)
		if len(ops) > 0 {
			cfg.Workers = 1 + int(ops[0]%4)
			cfg.QueueCapacity = int(ops[0] % 8)
			cfg.MaxJobWeight = 1 + int64(ops[0]%4)
			cfg.MaxInFlightWeight = cfg.MaxJobWeight * int64(cfg.Workers+cfg.QueueCapacity)
		}
		if len(ops) > 1 && ops[1]%2 == 1 {
			cfg.Ordering = OrderingPerKey
			cfg.MaxActiveKeys = 3
			cfg.PerKeyCapacity = 2
			cfg.KeyFailure = KeyFailureContinue
			if ops[1]%3 == 0 {
				cfg.KeyFailure = KeyFailureCancelPending
			}
		}
		cfg.Admission = AdmissionReject
		p, err := NewPool(cfg, func(ctx context.Context, n int) (int, error) {
			if n%7 == 0 {
				return 0, fmt.Errorf("task %d", n)
			}
			if n%7 == 1 {
				panic("fuzz panic")
			}
			return n, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		var futures []*Future[int]
		for i, op := range ops {
			switch op % 7 {
			case 0, 1:
				key := ""
				if cfg.Ordering == OrderingPerKey {
					key = fmt.Sprint(i % 3)
				}
				f, e := p.Submit(context.Background(), Job[int]{ID: fmt.Sprint(i % 8), Key: key, Weight: 1 + int64(i)%cfg.MaxJobWeight, Value: i})
				if e == nil {
					futures = append(futures, f)
				} else if !errors.Is(e, ErrSaturated) && !errors.Is(e, ErrDuplicateID) && !errors.Is(e, ErrClosed) {
					t.Fatal(e)
				}
			case 2:
				if len(futures) > 0 {
					futures[len(futures)-1].Cancel(nil)
				}
			case 3:
				assertInvariant(t, p.Stats(), cfg)
			case 5:
				if err := p.Shutdown(context.Background(), ShutdownDrain); err != nil {
					t.Fatal(err)
				}
			case 6:
				if err := p.Shutdown(context.Background(), ShutdownCancel); err != nil {
					t.Fatal(err)
				}
			case 4:
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				_, _ = p.Submit(ctx, Job[int]{ID: "canceled", Weight: 1})
			}
		}
		shutdownPool(t, p)
		for _, future := range futures {
			outcome(t, future)
		}
		assertInvariant(t, p.Stats(), cfg)
		stats := p.Stats()
		if stats.Accepted != uint64(len(futures)) || stats.InFlightWeight != 0 || stats.Queued != 0 || stats.Running != 0 {
			t.Fatalf("terminal model: %+v", stats)
		}
	})
}
