package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/retry"
)

type transport func(context.Context) (string, error)
type adapter struct{ send transport }

func (a adapter) Read(ctx context.Context) (string, error) { return a.send(ctx) }

func run() error {
	transient := errors.New("transient read failure")
	uncertain := errors.New("outcome unknown")
	cfg := retry.DefaultConfig()
	cfg.MaxRetries = 2
	cfg.InitialBackoff = time.Nanosecond
	cfg.MaxBackoff = time.Nanosecond
	cfg.Jitter = retry.JitterNone
	cfg.TotalTimeout = time.Second
	classify := func(err error) retry.Decision {
		if errors.Is(err, uncertain) {
			return retry.DecisionReconcile
		}
		if errors.Is(err, transient) {
			return retry.DecisionRetry
		}
		return retry.DecisionStop
	}
	for _, unknown := range []bool{false, true} {
		calls := 0
		var order []string
		a := adapter{send: func(ctx context.Context) (string, error) {
			if _, ok := ctx.Deadline(); !ok {
				return "", errors.New("missing budget")
			}
			calls++
			order = append(order, "attempt")
			if unknown {
				return "uncertain", uncertain
			}
			if calls < 3 {
				return "partial", transient
			}
			return "complete", nil
		}}
		middleware := func(next client.Handler[string]) client.Handler[string] {
			return func(ctx context.Context, info client.Invocation, op client.Operation[string]) (string, error) {
				order = append(order, "logical.before")
				value, err := next(ctx, info, op)
				order = append(order, "logical.after")
				return value, err
			}
		}
		value, err := client.Do(context.Background(), client.Invocation{Component: "memory", Operation: "Read"}, func(ctx context.Context) (string, error) { return retry.Do(ctx, cfg, classify, a.Read) }, middleware)
		expected := []string{"logical.before", "attempt", "attempt", "attempt", "logical.after"}
		if unknown {
			expected = []string{"logical.before", "attempt", "logical.after"}
			if calls != 1 || value != "uncertain" || !errors.Is(err, uncertain) || !errors.Is(err, retry.ErrReconciliationRequired) {
				return errors.New("uncertain result repeated")
			}
		} else if calls != 3 || value != "complete" || err != nil {
			return errors.New("transient retry failed")
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
	fmt.Println("retry composed with client: passed")
}
