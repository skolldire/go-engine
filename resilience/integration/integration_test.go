package integration

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/concurrency"
	"github.com/skolldire/go-engine/ratelimit"
	"github.com/skolldire/go-engine/resilience"
	"github.com/skolldire/go-engine/retry"
)

type memoryAdapter struct {
	send func(context.Context) (string, error)
}

func (a memoryAdapter) Read(ctx context.Context) (string, error) { return a.send(ctx) }
func TestMemoryTransportRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := errors.New("remote")
		behavior := original
		calls := 0
		c := circuitbreaker.DefaultConfig()
		c.Name = "memory"
		c.RequestThreshold = 1
		b, err := circuitbreaker.New(c, circuitbreaker.Dependencies{Classify: func(err error) bool { return errors.Is(err, original) }})
		if err != nil {
			t.Fatal(err)
		}
		l, err := concurrency.NewLimiter(concurrency.LimiterConfig{Capacity: 1})
		if err != nil {
			t.Fatal(err)
		}
		r, err := ratelimit.New(ratelimit.Config{RatePerSecond: 1, Burst: 2})
		if err != nil {
			t.Fatal(err)
		}
		cfg := resilience.DefaultConfig()
		cfg.Weight = 1
		cfg.Tokens = 1
		a := memoryAdapter{send: func(context.Context) (string, error) { calls++; return "partial", behavior }}
		fn, err := resilience.Wrap(a.Read, cfg, resilience.Dependencies{Breaker: b, Limiter: l, RateLimiter: r, Classify: func(err error) retry.Decision {
			if err != original {
				t.Error("local error leaked to classifier", err)
			}
			return retry.DecisionStop
		}})
		if err != nil {
			t.Fatal(err)
		}
		value, err := fn(context.Background())
		if value != "partial" || err != original || b.State() != circuitbreaker.StateOpen {
			t.Fatal(value, err, b.Stats())
		}
		value, err = fn(context.Background())
		if value != "" || (!errors.Is(err, circuitbreaker.ErrOpen) || !errors.Is(err, resilience.ErrRejected)) || calls != 1 || r.Allow(1) {
			t.Fatal(value, err, calls)
		}
		behavior = nil
		time.Sleep(c.OpenTimeout)
		value, err = fn(context.Background())
		if value != "partial" || err != nil || calls != 2 || b.State() != circuitbreaker.StateClosed {
			t.Fatal("recovery failed", value, err, calls, b.Stats())
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		release, err := l.Acquire(ctx, 1)
		if err != nil {
			t.Fatal("recovery leaked permit", err)
		}
		release()
	})
}
