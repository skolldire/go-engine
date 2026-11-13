package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/client"
	clientlogging "github.com/skolldire/go-engine/client/logging"
	"github.com/skolldire/go-engine/logging"
	"github.com/skolldire/go-engine/resilience"
	"github.com/skolldire/go-engine/retry"
)

func TestIndependentComposition(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		if err := verify(); err != nil {
			t.Fatal(err)
		}
	})
}
func TestLogicalDurationIncludesRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var output bytes.Buffer
		cfg := logging.DefaultConfig()
		cfg.Level = "debug"
		logger, err := logging.New(cfg, logging.Dependencies{Output: &output})
		if err != nil {
			t.Fatal(err)
		}
		observe, err := clientlogging.New[int](clientlogging.DefaultConfig(), clientlogging.Dependencies{Logger: logger})
		if err != nil {
			t.Fatal(err)
		}
		transient := errors.New("transient")
		calls := 0
		policy := resilience.DefaultConfig()
		policy.Retry.MaxRetries = 1
		policy.Retry.InitialBackoff = 2 * time.Second
		policy.Retry.MaxBackoff = 2 * time.Second
		policy.Retry.Jitter = retry.JitterNone
		op, err := resilience.Wrap(client.Operation[int](func(context.Context) (int, error) {
			calls++
			synctest.Sleep(time.Second)
			if calls == 1 {
				return 0, transient
			}
			return 7, nil
		}), policy, resilience.Dependencies{Classify: func(error) retry.Decision { return retry.DecisionRetry }})
		if err != nil {
			t.Fatal(err)
		}
		value, err := client.Do(context.Background(), client.Invocation{Component: "memory", Operation: "read"}, op, observe)
		if value != 7 || err != nil || calls != 2 {
			t.Fatal("retry fixture")
		}
		if bytes.Count(output.Bytes(), []byte("\n")) != 1 {
			t.Fatal("expected one logical completion")
		}
		var record map[string]any
		if err := json.Unmarshal(output.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record["duration"] != float64(4*time.Second) {
			t.Fatal("backoff or attempt missing from duration")
		}
	})
}
