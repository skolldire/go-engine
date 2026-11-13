package concurrency

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestPoolConfig(t *testing.T) {
	if err := DefaultPoolConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*PoolConfig){
		"workers":           func(c *PoolConfig) { c.Workers = 0 },
		"queue":             func(c *PoolConfig) { c.QueueCapacity = -1 },
		"admission":         func(c *PoolConfig) { c.Admission = "" },
		"admission_timeout": func(c *PoolConfig) { c.AdmissionTimeout = -1 },
		"task_timeout":      func(c *PoolConfig) { c.TaskTimeout = -1 },
		"weight":            func(c *PoolConfig) { c.MaxJobWeight = 0 },
		"budget":            func(c *PoolConfig) { c.MaxInFlightWeight = 0 },
		"ordering":          func(c *PoolConfig) { c.Ordering = "bad" },
		"unordered_keys":    func(c *PoolConfig) { c.MaxActiveKeys = 1 },
		"keyed_keys":        func(c *PoolConfig) { c.Ordering = OrderingPerKey },
		"keyed_queue":       func(c *PoolConfig) { c.Ordering = OrderingPerKey; c.MaxActiveKeys = 1 },
		"keyed_failure":     func(c *PoolConfig) { c.Ordering = OrderingPerKey; c.MaxActiveKeys = 1; c.PerKeyCapacity = 1 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c := DefaultPoolConfig()
			change(&c)
			err := c.Validate()
			var ce *ConfigError
			if !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field == "" {
				t.Fatalf("config error: %v", err)
			}
		})
	}
	c := DefaultPoolConfig()
	c.Ordering = OrderingPerKey
	c.MaxActiveKeys = 2
	c.PerKeyCapacity = 1
	c.KeyFailure = KeyFailureContinue
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPoolConfigSeededJSON(t *testing.T) {
	cases := []struct {
		name, input    string
		valid          bool
		workers, queue int
	}{
		{"omitted", "{}", true, 1, 0},
		{"explicit_zero_worker", `{"workers":0}`, false, 0, 0},
		{"explicit_zero_queue", `{"workers":4,"queue_capacity":0}`, true, 4, 0},
		{"invalid_negative", `{"queue_capacity":-1}`, false, 1, -1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cfg := DefaultPoolConfig()
			if err := json.Unmarshal([]byte(test.input), &cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Workers != test.workers || cfg.QueueCapacity != test.queue {
				t.Fatalf("decoded defaults: %+v", cfg)
			}
			if (cfg.Validate() == nil) != test.valid {
				t.Fatalf("validation: %+v", cfg)
			}
		})
	}
}
