package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/concurrency"
	"github.com/skolldire/go-engine/ratelimit"
	"github.com/skolldire/go-engine/resilience"
	"github.com/skolldire/go-engine/retry"
	"math"
	"reflect"
	"time"
)

type adapter struct {
	send func(context.Context) (string, error)
}

func (a adapter) Read(ctx context.Context) (string, error) { return a.send(ctx) }
func run() error {
	transient := errors.New("transient")
	uncertain := errors.New("uncertain")
	for _, scenario := range []string{"transient", "open", "uncertain"} {
		var order []string
		remoteClassified, callerClassified, calls := 0, 0, 0
		bcfg := circuitbreaker.DefaultConfig()
		bcfg.Name = "memory"
		if scenario == "open" {
			bcfg.RequestThreshold = 1
		}
		breaker, err := circuitbreaker.New(bcfg, circuitbreaker.Dependencies{Classify: func(err error) bool {
			remoteClassified++
			order = append(order, "breaker.classify")
			return errors.Is(err, transient)
		}})
		if err != nil {
			return err
		}
		limit, err := concurrency.NewLimiter(concurrency.LimiterConfig{Capacity: 1})
		if err != nil {
			return err
		}
		rate, err := ratelimit.New(ratelimit.Config{RatePerSecond: math.SmallestNonzeroFloat64, Burst: 3})
		if err != nil {
			return err
		}
		a := adapter{send: func(ctx context.Context) (string, error) {
			calls++
			order = append(order, "io")
			if _, ok := ctx.Deadline(); !ok || breaker.Stats().InFlight != 1 {
				return "", errors.New("missing timeout or breaker admission")
			}
			if scenario == "uncertain" {
				return "unknown", uncertain
			}
			if calls < 3 {
				return "partial", transient
			}
			return "complete", nil
		}}
		cfg := resilience.DefaultConfig()
		cfg.Retry.MaxRetries = 3
		cfg.Retry.InitialBackoff = time.Nanosecond
		cfg.Retry.MaxBackoff = time.Nanosecond
		cfg.Retry.Jitter = retry.JitterNone
		cfg.TotalTimeout = time.Second
		cfg.AttemptTimeout = time.Second / 2
		cfg.AdmissionTimeout = time.Second / 4
		cfg.Weight = 1
		cfg.Tokens = 1
		fn, err := resilience.Wrap(a.Read, cfg, resilience.Dependencies{Breaker: breaker, Limiter: limit, RateLimiter: rate, Classify: func(err error) retry.Decision {
			callerClassified++
			order = append(order, "retry.classify")
			if errors.Is(err, circuitbreaker.ErrOpen) || errors.Is(err, circuitbreaker.ErrProbeLimit) || errors.Is(err, resilience.ErrAdmission) {
				return "local classification should be intercepted"
			}
			if errors.Is(err, uncertain) {
				return retry.DecisionReconcile
			}
			return retry.DecisionRetry
		}})
		if err != nil {
			return err
		}
		middleware := func(next client.Handler[string]) client.Handler[string] {
			return func(ctx context.Context, info client.Invocation, op client.Operation[string]) (string, error) {
				order = append(order, "logical.before")
				value, err := next(ctx, info, op)
				order = append(order, "logical.after")
				return value, err
			}
		}
		value, err := client.Do(context.Background(), client.Invocation{Component: "memory", Operation: "Read"}, fn, middleware)
		expected := []string{"logical.before", "io", "breaker.classify", "retry.classify", "io", "breaker.classify", "retry.classify", "io", "logical.after"}
		spent := 3
		switch scenario {
		case "transient":
			if calls != 3 || value != "complete" || err != nil || remoteClassified != 2 || callerClassified != 2 {
				return errors.New("per-attempt composition")
			}
		case "open":
			spent = 2
			expected = []string{"logical.before", "io", "breaker.classify", "retry.classify", "logical.after"}
			if calls != 1 || value != "" || err != circuitbreaker.ErrOpen || callerClassified != 1 {
				return errors.New("local rejection retried")
			}
		case "uncertain":
			spent = 1
			expected = []string{"logical.before", "io", "breaker.classify", "retry.classify", "logical.after"}
			if calls != 1 || value != "unknown" || !errors.Is(err, retry.ErrReconciliationRequired) || !errors.Is(err, uncertain) {
				return errors.New("uncertain outcome repeated")
			}
		}
		if !reflect.DeepEqual(order, expected) {
			return errors.New("logical/attempt order")
		}
		if remaining := 3 - spent; remaining > 0 && !rate.Allow(remaining) {
			return errors.New("token accounting")
		}
		if rate.Allow(1) {
			return errors.New("tokens not charged before breaker")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		release, releaseErr := limit.Acquire(ctx, 1)
		cancel()
		if releaseErr != nil {
			return releaseErr
		}
		release()
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("resilience composed: all real tools, per-attempt accounting and reconciliation passed")
}
