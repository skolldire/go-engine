package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/skolldire/go-engine/retry"
)

func run() error {
	cfg := retry.DefaultConfig()
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := (retry.Config{}).Validate(); !errors.Is(err, retry.ErrInvalidConfig) {
		return errors.New("zero config accepted")
	}
	value, err := retry.Do(context.Background(), cfg, nil, func(context.Context) (int, error) { return 42, nil })
	if value != 42 || err != nil {
		return errors.New("success contract")
	}
	original := errors.New("domain")
	value, err = retry.Do(context.Background(), cfg, func(error) retry.Decision { return retry.DecisionStop }, func(context.Context) (int, error) { return 7, original })
	if value != 7 || err != original {
		return errors.New("stop identity")
	}
	calls := 0
	value, err = retry.Do(context.Background(), cfg, func(error) retry.Decision { return retry.DecisionReconcile }, func(context.Context) (int, error) { calls++; return 8, original })
	if value != 8 || calls != 1 || !errors.Is(err, retry.ErrReconciliationRequired) || !errors.Is(err, original) {
		return errors.New("reconcile contract")
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("caller canceled")
	cancel(cause)
	_, err = retry.Do(ctx, cfg, nil, func(context.Context) (int, error) { calls++; return 0, nil })
	if calls != 1 || !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		return errors.New("cancellation contract")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("retry minimal: passed")
}
