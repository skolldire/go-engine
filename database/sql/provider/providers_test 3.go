// Package provider_test verifies the SQL provider contract.
package provider_test

import (
	"context"
	"strings"
	"testing"

	sqlprovider "github.com/skolldire/go-engine/database/sql/provider/sql"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
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

func TestConfigKeyAndName(t *testing.T) {
	p := sqlprovider.New("main", sqlite.Open(":memory:"))
	assert.Equal(t, "sql_clients", p.ConfigKey())
	assert.True(t, strings.HasSuffix(p.Name(), ":main"), "got %q", p.Name())
	assert.Equal(t, p.Name(), p.Name(), "Name must be stable across calls")
}

// TestBuildsWithInjectedDriver is the point of this provider's shape: the driver
// comes from the caller, so the module does not link four GORM dialects.
func TestBuildsWithInjectedDriver(t *testing.T) {
	p := sqlprovider.New("main", sqlite.Open(":memory:"))
	raw := section(p.ConfigKey(), "main", map[string]any{
		"type":                 "sqlite",
		"max_open_connections": 4,
		"conn_max_lifetime":    "30s",
	})

	built, err := p.Init(context.Background(), raw, deps())

	require.NoError(t, err)
	assert.NotNil(t, built)
	assert.NoError(t, p.Close(context.Background()))
}

// TestNoDriverIsReported: a nil dialector is a programming error and must say so
// rather than fail deep inside GORM.
func TestNoDriverIsReported(t *testing.T) {
	p := sqlprovider.New("main", nil)
	raw := section(p.ConfigKey(), "main", map[string]any{"type": "sqlite"})

	_, err := p.Init(context.Background(), raw, deps())

	require.Error(t, err)
	assert.ErrorContains(t, err, "no driver given")
}

func TestMissingSectionIsReported(t *testing.T) {
	p := sqlprovider.New("main", sqlite.Open(":memory:"))
	_, err := p.Init(context.Background(), engine.NewMissingRawConfig(p.ConfigKey()), deps())
	assert.Error(t, err)
}

func TestMissingInstanceIsReported(t *testing.T) {
	p := sqlprovider.New("not-declared", sqlite.Open(":memory:"))
	raw := section(p.ConfigKey(), "other", map[string]any{})

	_, err := p.Init(context.Background(), raw, deps())

	require.Error(t, err)
	assert.ErrorContains(t, err, "not declared")
}

func TestUnknownKeyIsReported(t *testing.T) {
	p := sqlprovider.New("main", sqlite.Open(":memory:"))
	raw := section(p.ConfigKey(), "main", map[string]any{"typ": "sqlite"})

	_, err := p.Init(context.Background(), raw, deps())

	require.Error(t, err)
	assert.ErrorContains(t, err, "typ")
}

// TestBareNumberDurationIsReported covers the footgun end to end for this family.
func TestBareNumberDurationIsReported(t *testing.T) {
	p := sqlprovider.New("main", sqlite.Open(":memory:"))
	raw := section(p.ConfigKey(), "main", map[string]any{"conn_max_lifetime": 30})

	_, err := p.Init(context.Background(), raw, deps())

	require.Error(t, err)
	assert.ErrorContains(t, err, "not a duration")
}

func TestCloseBeforeInitIsSafe(t *testing.T) {
	p := sqlprovider.New("main", sqlite.Open(":memory:"))
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
