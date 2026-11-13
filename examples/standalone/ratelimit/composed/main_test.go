package main

import (
	"context"
	"errors"
	cb "github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/ratelimit"
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
func TestCanceledWaitDoesNotHoldProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		classified, calls := 0, 0
		cfg := cb.DefaultConfig()
		cfg.Name = "probe"
		cfg.RequestThreshold = 1
		b, err := cb.New(cfg, cb.Dependencies{Classify: func(error) bool { classified++; return true }})
		if err != nil {
			t.Fatal(err)
		}
		cb.Execute(context.Background(), b, func(context.Context) (string, error) { return "", errors.New("remote") })
		time.Sleep(cfg.OpenTimeout)
		if b.State() != cb.StateHalfOpen {
			t.Fatal(b.Stats())
		}
		l, err := ratelimit.New(ratelimit.Config{RatePerSecond: 1, Burst: 1})
		if err != nil {
			t.Fatal(err)
		}
		l.Allow(1)
		cause := errors.New("caller budget")
		ctx, cancel := context.WithTimeoutCause(context.Background(), 500*time.Millisecond, cause)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := client.Do(ctx, client.Invocation{Component: "memory", Operation: "Read"}, func(ctx context.Context) (string, error) {
				return retry.Do(ctx, retry.DefaultConfig(), nil, func(ctx context.Context) (string, error) {
					if err := l.Wait(ctx, 1); err != nil {
						return "", err
					}
					return cb.Execute(ctx, b, func(context.Context) (string, error) { calls++; return "ok", nil })
				})
			})
			done <- err
		}()
		synctest.Wait()
		if b.Stats().InFlight != 0 || classified != 1 {
			t.Fatal("probe held by rate wait", b.Stats())
		}
		time.Sleep(500 * time.Millisecond)
		err = <-done
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, cause) || calls != 0 || classified != 1 || b.Stats().Requests != 0 || l.Allow(1) {
			t.Fatal(err, calls, classified, b.Stats())
		}
	})
}
