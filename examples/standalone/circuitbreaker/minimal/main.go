package main

import (
	"context"
	"errors"
	"fmt"

	cb "github.com/skolldire/go-engine/circuitbreaker"
)

func run() error {
	cfg := cb.DefaultConfig()
	cfg.Name = "memory"
	cfg.RequestThreshold = 1
	if err := cfg.Validate(); err != nil {
		return err
	}
	original := errors.New("remote")
	business := errors.New("business")
	b, err := cb.New(cfg, cb.Dependencies{Classify: func(err error) bool { return errors.Is(err, original) }})
	if err != nil {
		return err
	}
	value, err := cb.Execute(context.Background(), b, func(context.Context) (int, error) { return 42, nil })
	if value != 42 || err != nil {
		return errors.New("success contract")
	}
	value, err = cb.Execute(context.Background(), b, func(context.Context) (int, error) { return 7, business })
	if value != 7 || err != business || b.Stats().Ignored != 1 {
		return errors.New("business classification")
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("caller canceled")
	cancel(cause)
	_, err = cb.Execute(ctx, b, func(context.Context) (int, error) { return 1, nil })
	if !errors.Is(err, cause) || !errors.Is(err, context.Canceled) || b.Stats().Requests != 2 {
		return errors.New("pre-cancel accounting")
	}
	value, err = cb.Execute(context.Background(), b, func(context.Context) (int, error) { return 3, original })
	if value != 3 || err != original || b.State() != cb.StateOpen {
		return errors.New("failure opening")
	}
	calls := 0
	value, err = cb.Execute(context.Background(), b, func(context.Context) (int, error) { calls++; return 1, nil })
	if value != 0 || err != cb.ErrOpen || calls != 0 {
		return errors.New("open transport executed")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("circuitbreaker minimal: passed; recovery verified by go test")
}
