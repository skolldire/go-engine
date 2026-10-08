// Package provider_test verifies the contract every AWS provider must satisfy.
//
// These tests live in the aws module rather than in a central place because a
// provider is only meaningful alongside the adapter it wraps: after the module
// split, a cross-family test could not cover this module's own statements.
package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/skolldire/go-engine/aws/provider/cognito"
	"github.com/skolldire/go-engine/aws/provider/dynamo"
	"github.com/skolldire/go-engine/aws/provider/s3"
	"github.com/skolldire/go-engine/aws/provider/ses"
	"github.com/skolldire/go-engine/aws/provider/sns"
	"github.com/skolldire/go-engine/aws/provider/sqs"
	"github.com/skolldire/go-engine/aws/provider/ssm"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// named builds every multi-instance AWS provider under one instance name.
func named(instance string) map[string]engine.Provider {
	return map[string]engine.Provider{
		"sqs":    sqs.New(instance),
		"sns":    sns.New(instance),
		"ses":    ses.New(instance),
		"s3":     s3.New(instance),
		"ssm":    ssm.New(instance),
		"dynamo": dynamo.New(instance),
	}
}

// validConfig is a minimal, valid section per adapter. The field names differ
// between adapters (sqs uses `endpoint`, sns `base_endpoint`, s3 `region` and
// `bucket`), which is exactly what the unknown-key check now enforces.
var validConfig = map[string]map[string]any{
	"sqs":    {"endpoint": "http://localhost:4566"},
	"sns":    {"base_endpoint": "http://localhost:4566"},
	"ses":    {"region": "us-east-1"},
	"s3":     {"region": "us-east-1", "bucket": "test-bucket"},
	"ssm":    {"region": "us-east-1"},
	"dynamo": {"endpoint": "http://localhost:4566", "table_prefix": "test"},
}

// expectedKeys pins the configuration section each provider consumes. These are
// the exact keys the pre-split typed configuration used; changing one silently
// stops reading an existing YAML file.
var expectedKeys = map[string]string{
	"sqs":    "sqs_clients",
	"sns":    "sns_clients",
	"ses":    "ses_clients",
	"s3":     "s3_clients",
	"ssm":    "ssm_clients",
	"dynamo": "dynamo_clients",
}

// withCredentials gives the AWS SDK something to resolve so provider Init can
// reach the client-construction step without touching the network.
func withCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-1")
}

func section(key, instance string, cfg map[string]any) engine.RawConfig {
	return engine.NewRawConfig(key, []any{map[string]any{instance: cfg}})
}

func deps() engine.Deps {
	return engine.Deps{
		Logger:    logger.NewService(logger.Config{Level: "error"}, nil),
		Telemetry: stubTelemetry{},
		Health:    stubHealth{},
		Section: func(key string) engine.RawConfig {
			if key == "aws" {
				return engine.NewRawConfig("aws", map[string]any{"region": "us-east-1"})
			}
			return engine.NewMissingRawConfig(key)
		},
		Resource: func(_ string, build func() (any, error)) (any, error) { return build() },
	}
}

func TestProviders_ConfigKeysMatchLegacyYAML(t *testing.T) {
	for label, p := range named("x") {
		t.Run(label, func(t *testing.T) {
			assert.Equal(t, expectedKeys[label], p.ConfigKey(),
				"changing this key stops reading existing configuration files")
		})
	}
	assert.Equal(t, "cognito", cognito.New().ConfigKey())
}

func TestProviders_NamesAreNamespacedAndStable(t *testing.T) {
	for label, p := range named("orders") {
		t.Run(label, func(t *testing.T) {
			name := p.Name()
			assert.True(t, strings.HasSuffix(name, ":orders"),
				"a named instance must carry its instance in the component name, got %q", name)
			assert.Equal(t, name, p.Name(), "Name must be stable across calls")
		})
	}
	assert.Equal(t, "cognito", cognito.New().Name())
}

// TestProviders_BuildTheirClient is the coverage that matters: it drives Init
// all the way to constructing the real SDK client.
func TestProviders_BuildTheirClient(t *testing.T) {
	withCredentials(t)

	for label, p := range named("orders") {
		t.Run(label, func(t *testing.T) {
			raw := section(p.ConfigKey(), "orders", validConfig[label])

			built, err := p.Init(context.Background(), raw, deps())

			require.NoError(t, err)
			assert.NotNil(t, built, "%s must contribute a client", label)
			assert.NoError(t, p.Close(context.Background()))
		})
	}
}

// TestProviders_ShareOneCredentialChain covers the memoised AWS config: the
// credential chain must be resolved once per engine, not once per adapter.
func TestProviders_ShareOneCredentialChain(t *testing.T) {
	withCredentials(t)

	var builds int
	d := deps()
	d.Resource = func(_ string, build func() (any, error)) (any, error) {
		builds++
		return build()
	}

	for label, p := range named("orders") {
		raw := section(p.ConfigKey(), "orders", validConfig[label])
		_, err := p.Init(context.Background(), raw, d)
		require.NoError(t, err)
	}

	assert.Equal(t, 6, builds,
		"each provider asks once; the engine's Resource memo is what collapses them")
}

func TestProviders_MissingInstanceIsReported(t *testing.T) {
	withCredentials(t)

	for label, p := range named("not-declared") {
		t.Run(label, func(t *testing.T) {
			raw := section(p.ConfigKey(), "something-else", map[string]any{})

			_, err := p.Init(context.Background(), raw, deps())
			require.Error(t, err, "a missing instance must not build a zero-value client")
			assert.ErrorContains(t, err, "not declared")
		})
	}
}

func TestProviders_MissingSectionIsReported(t *testing.T) {
	withCredentials(t)

	for label, p := range named("orders") {
		t.Run(label, func(t *testing.T) {
			_, err := p.Init(context.Background(), engine.NewMissingRawConfig(p.ConfigKey()), deps())
			assert.Error(t, err, "an absent section must be reported, not silently defaulted")
		})
	}
	_, err := cognito.New().Init(context.Background(), engine.NewMissingRawConfig("cognito"), deps())
	assert.Error(t, err)
}

// TestProviders_UnknownKeyIsReported guards the typo case end to end.
func TestProviders_UnknownKeyIsReported(t *testing.T) {
	withCredentials(t)

	p := sqs.New("orders")
	raw := section(p.ConfigKey(), "orders", map[string]any{"endpont": "typo"})

	_, err := p.Init(context.Background(), raw, deps())

	require.Error(t, err)
	assert.ErrorContains(t, err, "endpont")
}

func TestProviders_CloseBeforeInitIsSafe(t *testing.T) {
	all := named("x")
	all["cognito"] = cognito.New()

	for label, p := range all {
		t.Run(label, func(t *testing.T) {
			assert.NotPanics(t, func() {
				assert.NoError(t, p.Close(context.Background()))
			})
		})
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
