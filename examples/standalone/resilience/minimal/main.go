package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/skolldire/go-engine/resilience"
	"github.com/skolldire/go-engine/retry"
	"time"
)

func run() error {
	cfg := resilience.DefaultConfig()
	original := errors.New("domain")
	parent := context.Background()
	fn, err := resilience.Wrap(func(ctx context.Context) (int, error) {
		if ctx != parent {
			return 0, errors.New("context changed")
		}
		return 7, original
	}, cfg, resilience.Dependencies{})
	if err != nil {
		return err
	}
	value, err := fn(parent)
	if value != 7 || err != original {
		return errors.New("result/error identity")
	}
	if _, err := fn(nil); !errors.Is(err, resilience.ErrInvalidCall) {
		return errors.New("nil context accepted")
	}
	if _, err := resilience.Wrap[int](nil, cfg, resilience.Dependencies{}); !errors.Is(err, resilience.ErrInvalidCall) {
		return errors.New("nil operation accepted")
	}
	cause := errors.New("caller canceled")
	ctx, cancel := context.WithCancelCause(parent)
	cancel(cause)
	if _, err := fn(ctx); !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		return errors.New("cancellation cause")
	}
	cfg.Retry.MaxRetries = 1
	cfg.Retry.InitialBackoff = time.Nanosecond
	cfg.Retry.MaxBackoff = time.Nanosecond
	cfg.Retry.Jitter = retry.JitterNone
	calls := 0
	fn, err = resilience.Wrap(func(context.Context) (int, error) {
		calls++
		if calls == 1 {
			return 1, original
		}
		return 42, nil
	}, cfg, resilience.Dependencies{Classify: func(error) retry.Decision { return retry.DecisionRetry }})
	if err != nil {
		return err
	}
	value, err = fn(parent)
	if value != 42 || err != nil || calls != 2 {
		return errors.New("explicit retry composition")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("resilience minimal: typed boundary and explicit retry passed (three-module library graph)")
}
