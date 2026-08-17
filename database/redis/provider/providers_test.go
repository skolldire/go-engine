// Package provider_test verifies the Redis provider contract in the module
// that owns the adapter it wraps.
package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/skolldire/go-engine/database/redis/provider/redis"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deps() engine.Deps {
	return engine.Deps{
		Logger:    logger.NewService(logger.Config{Level: "error"}, nil),
		Telemetry: stubTelemetry{},
		Health:    stubHealth{},
		Section:   func(k string) engine.RawConfig { return engine.NewMissingRawConfig(k) },
		Resource:  func(_ string, b func() (any, error)) (any, error) { return b() },
	}
}

func section(key, instance string, cfg map[string]any) engine.RawConfig {
	return engine.NewRawConfig(key, []any{map[string]any{instance: cfg}})
}

// TestConfigKey pins the YAML section: changing it stops reading existing files.
func TestConfigKey(t *testing.T) {
	assert.Equal(t, "redis_clients", redis.New("x").ConfigKey())
}

func TestNameIsNamespaced(t *testing.T) {
	p := redis.New("cache")
	assert.True(t, strings.HasSuffix(p.Name(), ":cache"), "got %q", p.Name())
	assert.Equal(t, p.Name(), p.Name(), "Name must be stable across calls")
}

func TestMissingSectionIsReported(t *testing.T) {
	p := redis.New("cache")
	_, err := p.Init(context.Background(), engine.NewMissingRawConfig(p.ConfigKey()), deps())
	assert.Error(t, err, "an absent section must be reported, not silently defaulted")
}

func TestMissingInstanceIsReported(t *testing.T) {
	p := redis.New("not-declared")
	raw := section(p.ConfigKey(), "something-else", map[string]any{})

	_, err := p.Init(context.Background(), raw, deps())

	require.Error(t, err)
	assert.ErrorContains(t, err, "not declared")
}

// TestUnknownKeyIsReported guards the typo case: a misspelled key used to leave
// the field at its zero value and start the client against the wrong target.
func TestUnknownKeyIsReported(t *testing.T) {
	p := redis.New("cache")
	raw := section(p.ConfigKey(), "cache", map[string]any{"hostt": "x"})

	_, err := p.Init(context.Background(), raw, deps())

	require.Error(t, err)
	assert.ErrorContains(t, err, "hostt")
}

// TestUnreachableTargetIsReported: a client that cannot connect must fail the
// build rather than hand back a broken component.
func TestUnreachableTargetIsReported(t *testing.T) {
	p := redis.New("cache")
	raw := section(p.ConfigKey(), "cache", map[string]any{"host": "127.0.0.1", "port": 1, "timeout": "1s"})

	_, err := p.Init(context.Background(), raw, deps())
	assert.Error(t, err)
}

func TestCloseBeforeInitIsSafe(t *testing.T) {
	p := redis.New("cache")
	assert.NotPanics(t, func() { assert.NoError(t, p.Close(context.Background())) })
}

type stubTelemetry struct{}

func (stubTelemetry) Span(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}
func (stubTelemetry) Counter(context.Context, string, int64)     {}
func (stubTelemetry) Histogram(context.Context, string, float64) {}

type stubHealth struct{}

func (stubHealth) RegisterCheck(string, func(context.Context) error) {}
