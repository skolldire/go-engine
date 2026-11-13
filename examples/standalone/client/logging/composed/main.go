package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/client"
	clientlogging "github.com/skolldire/go-engine/client/logging"
	"github.com/skolldire/go-engine/concurrency"
	"github.com/skolldire/go-engine/logging"
	"github.com/skolldire/go-engine/ratelimit"
	"github.com/skolldire/go-engine/resilience"
	"github.com/skolldire/go-engine/retry"
)

type adapter struct {
	transport func(context.Context) (int, error)
}

func (a adapter) Read(ctx context.Context) (int, error) { return a.transport(ctx) }
func verify() error {
	var out bytes.Buffer
	loggerConfig := logging.DefaultConfig()
	loggerConfig.Level = "debug"
	logger, err := logging.New(loggerConfig, logging.Dependencies{Output: &out})
	if err != nil {
		return err
	}
	observe, err := clientlogging.New[int](clientlogging.DefaultConfig(), clientlogging.Dependencies{Logger: logger})
	if err != nil {
		return err
	}
	transient := errors.New("private transient error")
	uncertain := errors.New("private uncertain commit")
	breakerConfig := circuitbreaker.DefaultConfig()
	breakerConfig.Name = "memory"
	breaker, err := circuitbreaker.New(breakerConfig, circuitbreaker.Dependencies{Classify: func(err error) bool { return errors.Is(err, transient) }})
	if err != nil {
		return err
	}
	limiter, err := concurrency.NewLimiter(concurrency.LimiterConfig{Capacity: 2})
	if err != nil {
		return err
	}
	rate, err := ratelimit.New(ratelimit.Config{RatePerSecond: 1000, Burst: 16})
	if err != nil {
		return err
	}
	cfg := resilience.DefaultConfig()
	cfg.Retry.MaxRetries = 2
	cfg.Retry.InitialBackoff = time.Nanosecond
	cfg.Retry.MaxBackoff = time.Nanosecond
	cfg.Retry.Jitter = retry.JitterNone
	cfg.Weight = 1
	cfg.Tokens = 1
	calls := 0
	unknown := false
	transport := adapter{transport: func(context.Context) (int, error) {
		calls++
		if unknown {
			return 99, uncertain
		}
		if calls < 3 {
			return 17, transient
		}
		return 17, nil
	}}
	operation, err := resilience.Wrap(client.Operation[int](transport.Read), cfg, resilience.Dependencies{Breaker: breaker, Limiter: limiter, RateLimiter: rate, Classify: func(err error) retry.Decision {
		if errors.Is(err, uncertain) {
			return retry.DecisionReconcile
		}
		if errors.Is(err, transient) {
			return retry.DecisionRetry
		}
		return retry.DecisionStop
	}})
	if err != nil {
		return err
	}
	info := client.Invocation{Component: "memory", Operation: "read"}
	value, err := client.Do(context.Background(), info, operation, observe)
	if err != nil || value != 17 || calls != 3 {
		return errors.New("transient retry failed")
	}
	if err := checkRecord(out.Bytes(), true); err != nil {
		return err
	}
	out.Reset()
	unknown = true
	value, err = client.Do(context.Background(), info, operation, observe)
	if value != 99 || calls != 4 || !errors.Is(err, retry.ErrReconciliationRequired) || !errors.Is(err, uncertain) {
		return errors.New("uncertain operation repeated or cause lost")
	}
	if err := checkRecord(out.Bytes(), false); err != nil {
		return err
	}
	release, err := limiter.Acquire(context.Background(), 2)
	if err != nil {
		return err
	}
	release()
	stats := breaker.Stats()
	if stats.Requests != 4 || stats.InFlight != 0 {
		return errors.New("per-attempt breaker counts changed")
	}
	return nil
}
func checkRecord(data []byte, success bool) error {
	if bytes.Count(data, []byte("\n")) != 1 {
		return errors.New("expected one logical completion")
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	if record["component"] != "memory" || record["operation"] != "read" || record["success"] != success || record["duration"] == nil || bytes.Contains(data, []byte("private")) {
		return errors.New("unsafe or incomplete logical observation")
	}
	return nil
}
func main() {
	if err := verify(); err != nil {
		panic(err)
	}
	fmt.Println("client/logging composed: one logical observation, real resilience and reconciliation passed")
}
