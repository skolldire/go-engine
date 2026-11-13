package circuitbreaker_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	cb "github.com/skolldire/go-engine/circuitbreaker"
)

type response struct {
	Status int
	Body   string
}
type memoryAdapter struct {
	send func(context.Context) (response, error)
}

func (a memoryAdapter) Read(ctx context.Context, b *cb.Breaker) (response, error) {
	return cb.Execute(ctx, b, a.send)
}
func TestMemoryTransportLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		remote := errors.New("remote unavailable")
		business := errors.New("business conflict")
		cfg := cb.DefaultConfig()
		cfg.Name = "memory"
		cfg.RequestThreshold = 1
		cfg.HalfOpenMax = 2
		b, err := cb.New(cfg, cb.Dependencies{Classify: func(err error) bool { return errors.Is(err, remote) }})
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		behavior := business
		a := memoryAdapter{send: func(context.Context) (response, error) { calls++; return response{200, "partial"}, behavior }}
		value, err := a.Read(context.Background(), b)
		if err != business || value.Body != "partial" || b.Stats().Ignored != 1 {
			t.Fatal(value, err, b.Stats())
		}
		behavior = nil
		a.Read(context.Background(), b)
		behavior = remote
		a.Read(context.Background(), b)
		if b.State() != cb.StateOpen {
			t.Fatal(b.Stats())
		}
		value, err = a.Read(context.Background(), b)
		if calls != 3 || err != cb.ErrOpen || value != (response{}) {
			t.Fatal(calls, value, err)
		}
		time.Sleep(cfg.OpenTimeout)
		behavior = nil
		a.Read(context.Background(), b)
		if b.State() != cb.StateHalfOpen {
			t.Fatal(b.Stats())
		}
		behavior = business
		a.Read(context.Background(), b)
		if b.Stats().Successes != 1 {
			t.Fatal(b.Stats())
		}
		behavior = nil
		a.Read(context.Background(), b)
		if b.State() != cb.StateClosed || calls != 6 {
			t.Fatal(b.Stats(), calls)
		}
	})
}
