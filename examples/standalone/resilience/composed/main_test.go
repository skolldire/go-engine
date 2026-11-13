package main

import (
	"context"
	"errors"
	"github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/concurrency"
	"github.com/skolldire/go-engine/ratelimit"
	"github.com/skolldire/go-engine/resilience"
	"github.com/skolldire/go-engine/retry"
	"testing"
	"testing/synctest"
	"time"
)

func TestConsumer(t *testing.T) {
	if err := run(); err != nil {
		t.Fatal(err)
	}
}
func TestAdmissionDoesNotHoldProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := resilience.DefaultConfig()
		cfg.Weight = 1
		cfg.Tokens = 1
		cfg.TotalTimeout = 2 * time.Second
		cfg.AdmissionTimeout = 500 * time.Millisecond
		cfg.AttemptTimeout = 100 * time.Millisecond
		cfg.Retry.MaxRetries = 2
		lim, err := concurrency.NewLimiter(concurrency.LimiterConfig{Capacity: 1})
		if err != nil {
			t.Fatal(err)
		}
		rate, err := ratelimit.New(ratelimit.Config{RatePerSecond: 1, Burst: 1})
		if err != nil {
			t.Fatal(err)
		}
		bcfg := circuitbreaker.DefaultConfig()
		bcfg.Name = "probe"
		bcfg.RequestThreshold = 1
		classified := 0
		b, err := circuitbreaker.New(bcfg, circuitbreaker.Dependencies{Classify: func(error) bool { classified++; return true }})
		if err != nil {
			t.Fatal(err)
		}
		circuitbreaker.Execute(context.Background(), b, func(context.Context) (int, error) { return 0, errors.New("remote") })
		time.Sleep(30 * time.Second)
		rate.Allow(1)
		fn, err := resilience.Wrap(func(context.Context) (int, error) { t.Error("admission reached transport"); return 0, nil }, cfg, resilience.Dependencies{Limiter: lim, RateLimiter: rate, Breaker: b, Classify: func(error) retry.Decision { t.Error("admission reached caller classifier"); return retry.DecisionRetry }})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := fn(context.Background()); done <- err }()
		synctest.Wait()
		if b.Stats().InFlight != 0 || b.State() != circuitbreaker.StateHalfOpen {
			t.Fatal(b.Stats())
		}
		probeCtx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		release, err := lim.Acquire(probeCtx, 1)
		cancel()
		if err != nil {
			t.Fatal("permit held waiting rate", err)
		}
		release()
		time.Sleep(500 * time.Millisecond)
		err = <-done
		if !errors.Is(err, resilience.ErrAdmission) || !errors.Is(err, resilience.ErrAdmissionTimeout) || classified != 1 || b.Stats().Requests != 0 {
			t.Fatal(err, classified, b.Stats())
		}
	})
}
