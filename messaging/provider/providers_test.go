// Package provider_test verifies the contract every messaging provider must
// satisfy, in the module that owns the adapters they wrap.
package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/skolldire/go-engine/messaging/provider/grpcclient"
	"github.com/skolldire/go-engine/messaging/provider/grpcserver"
	"github.com/skolldire/go-engine/messaging/provider/kafka"
	"github.com/skolldire/go-engine/messaging/provider/rabbitmq"
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

// TestConfigKeys pins the YAML sections; changing one stops reading existing files.
func TestConfigKeys(t *testing.T) {
	assert.Equal(t, "grpc_client", grpcclient.New("x").ConfigKey())
	assert.Equal(t, "rabbitmq_clients", rabbitmq.New("x").ConfigKey())
	assert.Equal(t, "kafka", kafka.New().ConfigKey())
	assert.Equal(t, "grpc_server", grpcserver.New().ConfigKey())
}

func TestNamesAreNamespaced(t *testing.T) {
	for _, p := range []engine.Provider{grpcclient.New("auth"), rabbitmq.New("auth")} {
		assert.True(t, strings.HasSuffix(p.Name(), ":auth"), "got %q", p.Name())
		assert.Equal(t, p.Name(), p.Name(), "Name must be stable")
	}
	assert.Equal(t, "kafka", kafka.New().Name())
	assert.Equal(t, "grpc_server", grpcserver.New().Name())
}

// TestGRPCClient_BuildsLazily covers the fix that made connections lazy: the
// provider must not block on an unreachable target.
func TestGRPCClient_BuildsLazily(t *testing.T) {
	p := grpcclient.New("auth")
	raw := section(p.ConfigKey(), "auth", map[string]any{
		"target":  "127.0.0.1:1",
		"timeout": "2s",
	})

	built, err := p.Init(context.Background(), raw, deps())

	require.NoError(t, err, "a lazy gRPC connection must not fail on an unreachable target")
	assert.NotNil(t, built)
	assert.NoError(t, p.Close(context.Background()))
}

func TestGRPCServer_Builds(t *testing.T) {
	p := grpcserver.New()
	raw := engine.NewRawConfig(p.ConfigKey(), map[string]any{"puerto": 50099})

	built, err := p.Init(context.Background(), raw, deps())

	require.NoError(t, err)
	assert.NotNil(t, built)
	assert.NoError(t, p.Close(context.Background()))
}

func TestMissingSectionIsReported(t *testing.T) {
	for _, p := range []engine.Provider{
		grpcclient.New("x"), rabbitmq.New("x"), kafka.New(), grpcserver.New(),
	} {
		_, err := p.Init(context.Background(), engine.NewMissingRawConfig(p.ConfigKey()), deps())
		assert.Error(t, err, "%s must report an absent section", p.Name())
	}
}

func TestMissingInstanceIsReported(t *testing.T) {
	for _, p := range []engine.Provider{grpcclient.New("nope"), rabbitmq.New("nope")} {
		raw := section(p.ConfigKey(), "other", map[string]any{})
		_, err := p.Init(context.Background(), raw, deps())
		require.Error(t, err)
		assert.ErrorContains(t, err, "not declared")
	}
}

func TestUnknownKeyIsReported(t *testing.T) {
	p := grpcclient.New("auth")
	raw := section(p.ConfigKey(), "auth", map[string]any{"targt": "typo"})

	_, err := p.Init(context.Background(), raw, deps())

	require.Error(t, err)
	assert.ErrorContains(t, err, "targt")
}

func TestCloseBeforeInitIsSafe(t *testing.T) {
	for _, p := range []engine.Provider{
		grpcclient.New("x"), rabbitmq.New("x"), kafka.New(), grpcserver.New(),
	} {
		assert.NotPanics(t, func() { assert.NoError(t, p.Close(context.Background())) })
	}
}

type stubTelemetry struct{}

func (stubTelemetry) Span(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}
func (stubTelemetry) Counter(context.Context, string, int64)     {}
func (stubTelemetry) Histogram(context.Context, string, float64) {}

type stubHealth struct{}

func (stubHealth) RegisterCheck(string, func(context.Context) error) {}
