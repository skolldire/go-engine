package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/skolldire/go-engine/ratelimit"
)

func run() error {
	cfg := ratelimit.Config{RatePerSecond: 1, Burst: 3}
	if err := cfg.Validate(); err != nil {
		return err
	}
	l, err := ratelimit.New(cfg)
	if err != nil {
		return err
	}
	if !l.Allow(2) {
		return errors.New("initial burst")
	}
	if err := l.Wait(context.Background(), 1); err != nil {
		return err
	}
	if l.Allow(1) || l.Allow(0) {
		return errors.New("invalid/excess admission")
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("caller canceled")
	cancel(cause)
	err = l.Wait(ctx, 1)
	if !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		return errors.New("cancellation cause lost")
	}
	if err := l.Wait(context.Background(), 4); !errors.Is(err, ratelimit.ErrInvalidCall) {
		return errors.New("invalid weight accepted")
	}
	if _, err := ratelimit.New(ratelimit.Config{}); !errors.Is(err, ratelimit.ErrInvalidConfig) {
		return errors.New("implicit defaults")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("ratelimit minimal: passed; timed refill/FIFO checked by go test")
}
