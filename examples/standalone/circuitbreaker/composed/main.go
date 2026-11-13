package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	cb "github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/retry"
)

type adapter struct {
	send func(context.Context) (string, error)
}

func (a adapter) Read(ctx context.Context) (string, error) { return a.send(ctx) }
func run() error {
	transient := errors.New("transient")
	uncertain := errors.New("outcome unknown")
	classify := func(err error) retry.Decision {
		if errors.Is(err, cb.ErrOpen) || errors.Is(err, cb.ErrProbeLimit) {
			return retry.DecisionStop
		}
		if errors.Is(err, uncertain) {
			return retry.DecisionReconcile
		}
		if errors.Is(err, transient) {
			return retry.DecisionRetry
		}
		return retry.DecisionStop
	}
	if classify(cb.ErrProbeLimit) != retry.DecisionStop {
		return errors.New("probe limit retry policy")
	}
	for _, scenario := range []string{"transient", "open", "uncertain"} {
		cfg := cb.DefaultConfig()
		cfg.Name = "memory"
		if scenario == "open" {
			cfg.RequestThreshold = 1
		}
		breaker, err := cb.New(cfg, cb.Dependencies{Classify: func(err error) bool { return errors.Is(err, transient) }})
		if err != nil {
			return err
		}
		rcfg := retry.DefaultConfig()
		rcfg.MaxRetries = 3
		rcfg.InitialBackoff = time.Nanosecond
		rcfg.MaxBackoff = time.Nanosecond
		rcfg.Jitter = retry.JitterNone
		rcfg.TotalTimeout = time.Second
		calls := 0
		var order []string
		a := adapter{send: func(ctx context.Context) (string, error) {
			calls++
			order = append(order, "transport")
			if _, ok := ctx.Deadline(); !ok {
				return "", errors.New("missing retry budget")
			}
			if scenario == "uncertain" {
				return "unknown", uncertain
			}
			if calls <= 2 {
				return "partial", transient
			}
			return "complete", nil
		}}
		middleware := func(next client.Handler[string]) client.Handler[string] {
			return func(ctx context.Context, inv client.Invocation, op client.Operation[string]) (string, error) {
				order = append(order, "logical.before")
				value, err := next(ctx, inv, op)
				order = append(order, "logical.after")
				return value, err
			}
		}
		value, err := client.Do(context.Background(), client.Invocation{Component: "memory", Operation: "Read"}, func(ctx context.Context) (string, error) {
			return retry.Do(ctx, rcfg, classify, func(ctx context.Context) (string, error) {
				order = append(order, "attempt")
				return cb.Execute(ctx, breaker, a.Read)
			})
		}, middleware)
		expected := []string{"logical.before", "attempt", "transport", "attempt", "transport", "attempt", "transport", "logical.after"}
		switch scenario {
		case "transient":
			if calls != 3 || value != "complete" || err != nil {
				return errors.New("transient recovery")
			}
		case "open":
			expected = []string{"logical.before", "attempt", "transport", "attempt", "logical.after"}
			if calls != 1 || err != cb.ErrOpen || value != "" {
				return errors.New("open caused remote retries")
			}
		case "uncertain":
			expected = []string{"logical.before", "attempt", "transport", "logical.after"}
			if calls != 1 || value != "unknown" || !errors.Is(err, retry.ErrReconciliationRequired) || !errors.Is(err, uncertain) || breaker.Stats().Ignored != 1 {
				return errors.New("uncertain outcome repeated")
			}
		}
		if !reflect.DeepEqual(order, expected) {
			return errors.New("logical/attempt ordering")
		}
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("circuitbreaker composed: client + retry + breaker passed")
}
