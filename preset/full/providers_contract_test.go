// The contract tests below validate what every provider must satisfy: a stable
// name, the configuration key it claims, correct decoding of its own section,
// and a Close that is safe on a provider that was never initialised. A provider
// that violates any of these breaks the engine at a consumer's startup.
//
// They live in the full preset because it is the only module that depends on
// every adapter family; from any family module half the providers are invisible.
package full_test

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
	"github.com/skolldire/go-engine/database/memcached/provider/memcached"
	"github.com/skolldire/go-engine/database/mongodb/provider/mongodb"
	"github.com/skolldire/go-engine/database/redis/provider/redis"
	"github.com/skolldire/go-engine/http/provider/rest"
	"github.com/skolldire/go-engine/messaging/provider/grpcclient"
	"github.com/skolldire/go-engine/messaging/provider/grpcserver"
	"github.com/skolldire/go-engine/messaging/provider/kafka"
	"github.com/skolldire/go-engine/messaging/provider/rabbitmq"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/provider/otel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// named builds every multi-instance provider under a common instance name.
func named(instance string) map[string]engine.Provider {
	return map[string]engine.Provider{
		"sqs":        sqs.New(instance),
		"sns":        sns.New(instance),
		"ses":        ses.New(instance),
		"s3":         s3.New(instance),
		"ssm":        ssm.New(instance),
		"dynamo":     dynamo.New(instance),
		"redis":      redis.New(instance),
		"memcached":  memcached.New(instance),
		"mongodb":    mongodb.New(instance),
		"rabbitmq":   rabbitmq.New(instance),
		"rest":       rest.New(instance),
		"grpcclient": grpcclient.New(instance),
	}
}

func singletons() map[string]engine.Provider {
	return map[string]engine.Provider{
		"cognito":    cognito.New(),
		"kafka":      kafka.New(),
		"grpcserver": grpcserver.New(),
		"otel":       otel.New(),
	}
}

// expectedKeys pins the configuration section each provider consumes. These are
// the exact keys the previous typed configuration used; a change here silently
// stops reading a consumer's existing YAML.
var expectedKeys = map[string]string{
	"sqs":        "sqs_clients",
	"sns":        "sns_clients",
	"ses":        "ses_clients",
	"s3":         "s3_clients",
	"ssm":        "ssm_clients",
	"dynamo":     "dynamo_clients",
	"redis":      "redis_clients",
	"memcached":  "memcached_clients",
	"mongodb":    "mongodb_clients",
	"rabbitmq":   "rabbitmq_clients",
	"rest":       "rest",
	"grpcclient": "grpc_client",
	"cognito":    "cognito",
	"kafka":      "kafka",
	"grpcserver": "grpc_server",
	"otel":       "telemetry",
}

func TestProviders_ConfigKeysMatchLegacyYAML(t *testing.T) {
	all := named("x")
	for k, v := range singletons() {
		all[k] = v
	}

	for label, p := range all {
		t.Run(label, func(t *testing.T) {
			want, ok := expectedKeys[label]
			require.True(t, ok, "no expected key declared for %s", label)
			assert.Equal(t, want, p.ConfigKey(),
				"changing this key stops reading existing configuration files")
		})
	}
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

	for label, p := range singletons() {
		t.Run(label, func(t *testing.T) {
			assert.NotEmpty(t, p.Name())
			assert.Equal(t, p.Name(), p.Name())
		})
	}
}

// TestProviders_CloseBeforeInitIsSafe matters because the engine closes every
// registered provider during rollback, including ones whose Init never ran to
// completion.
func TestProviders_CloseBeforeInitIsSafe(t *testing.T) {
	all := named("x")
	for k, v := range singletons() {
		all[k] = v
	}

	for label, p := range all {
		t.Run(label, func(t *testing.T) {
			assert.NotPanics(t, func() {
				assert.NoError(t, p.Close(context.Background()))
			})
		})
	}
}

// TestProviders_MissingInstanceIsReported guards the failure mode a consumer
// hits most: asking for a client the YAML does not declare.
func TestProviders_MissingInstanceIsReported(t *testing.T) {
	for label, p := range named("not-declared") {
		t.Run(label, func(t *testing.T) {
			raw := engine.NewRawConfig(p.ConfigKey(), []any{
				map[string]any{"something-else": map[string]any{}},
			})

			_, err := p.Init(context.Background(), raw, deps())
			require.Error(t, err, "a missing instance must not build a zero-value client")
			assert.Contains(t, err.Error(), "not declared")
		})
	}
}

// TestProviders_MissingSectionIsReported covers a section absent entirely.
func TestProviders_MissingSectionIsReported(t *testing.T) {
	all := named("x")
	for k, v := range singletons() {
		if k == "otel" {
			continue // telemetry is valid with defaults and no section
		}
		all[k] = v
	}

	for label, p := range all {
		t.Run(label, func(t *testing.T) {
			_, err := p.Init(context.Background(), engine.NewMissingRawConfig(p.ConfigKey()), deps())
			assert.Error(t, err, "an absent section must be reported, not silently defaulted")
		})
	}
}

// TestOtel_RejectsRenamedLegacyKeys is the regression test for the silent
// migration trap: the old telemetry section used otel_endpoint/sample_rate,
// which decode to empty values under the new names while still reporting
// telemetry as enabled.
func TestOtel_RejectsRenamedLegacyKeys(t *testing.T) {
	cases := map[string]string{
		"otel_endpoint": "exporter_endpoint",
		"sample_rate":   "sampling_rate",
	}

	for oldKey, newKey := range cases {
		t.Run(oldKey, func(t *testing.T) {
			raw := engine.NewRawConfig("telemetry", map[string]any{
				"service_name": "svc",
				oldKey:         "value",
			})

			_, err := otel.New().Init(context.Background(), raw, deps())
			require.Error(t, err)
			assert.Contains(t, err.Error(), oldKey)
			assert.Contains(t, err.Error(), newKey)
		})
	}
}

// deps builds the minimal Deps a provider needs to reach its decode step.
func deps() engine.Deps {
	return engine.Deps{
		Logger:    logger.NewService(logger.Config{Level: "error"}, nil),
		Telemetry: stubTelemetry{},
		Health:    stubHealth{},
		Section:   func(string) engine.RawConfig { return engine.NewMissingRawConfig("") },
		Resource: func(_ string, build func() (any, error)) (any, error) {
			return build()
		},
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
