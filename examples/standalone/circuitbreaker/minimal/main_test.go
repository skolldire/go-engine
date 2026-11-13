package main

import (
	"context"
	"errors"
	cb "github.com/skolldire/go-engine/circuitbreaker"
	"testing"
	"testing/synctest"
	"time"
)

func TestConsumer(t *testing.T) {
	if err := run(); err != nil {
		t.Fatal(err)
	}
}
func TestRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := cb.DefaultConfig()
		cfg.Name = "recovery"
		cfg.RequestThreshold = 1
		cfg.HalfOpenMax = 2
		b, err := cb.New(cfg, cb.Dependencies{Classify: func(error) bool { return true }})
		if err != nil {
			t.Fatal(err)
		}
		cb.Execute(context.Background(), b, func(context.Context) (int, error) { return 5, errors.New("remote") })
		time.Sleep(cfg.OpenTimeout)
		if b.State() != cb.StateHalfOpen {
			t.Fatal(b.Stats())
		}
		for range 2 {
			value, err := cb.Execute(context.Background(), b, func(context.Context) (int, error) { return 7, nil })
			if value != 7 || err != nil {
				t.Fatal(value, err)
			}
		}
		if b.Stats() != (cb.Counts{State: cb.StateClosed}) {
			t.Fatal(b.Stats())
		}
	})
}
