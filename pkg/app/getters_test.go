package app

import (
	"context"
	"testing"

	"github.com/skolldire/go-engine/aws/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	"github.com/skolldire/go-engine/pkg/config/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEngine_GettersAreNilSafeOnEmptyEngine covers the whole getter surface
// against a zero-value Engine. Every one of them must return the zero value
// rather than panicking on a nil registry, which is what a consumer calling a
// getter before Build (or after a failed Build) actually does.
func TestEngine_GettersAreNilSafeOnEmptyEngine(t *testing.T) {
	e := &Engine{}

	assert.NotPanics(t, func() {
		assert.Nil(t, e.GetRouter())
		assert.Nil(t, e.GetLogger())
		assert.Nil(t, e.GetConfig())
		assert.Nil(t, e.GetTelemetry())
		assert.Nil(t, e.GetGRPCServer())
		assert.Nil(t, e.GetValidator())
		assert.Nil(t, e.GetCloudClient())
		assert.Nil(t, e.GetCognito())
		assert.Nil(t, e.GetKafkaProducer())
		assert.Nil(t, e.GetKafkaConsumer())
		assert.Nil(t, e.GetHealthService())
		assert.Nil(t, e.GetOTELProvider())
		assert.Nil(t, e.GetFeatureFlags())
		assert.Nil(t, e.GetSQSClient())
		assert.Nil(t, e.GetSNSClient())
		assert.Nil(t, e.GetDynamoDBClient())
		assert.Nil(t, e.GetRedisClient())
		assert.Empty(t, e.GetErrors())
	})

	assert.NotPanics(t, func() {
		assert.Nil(t, e.GetRestClient("missing"))
		assert.Nil(t, e.GetGRPCClient("missing"))
		assert.Nil(t, e.GetSQSClientByName("missing"))
		assert.Nil(t, e.GetSNSClientByName("missing"))
		assert.Nil(t, e.GetDynamoDBClientByName("missing"))
		assert.Nil(t, e.GetRedisClientByName("missing"))
		assert.Nil(t, e.GetSSMClientByName("missing"))
		assert.Nil(t, e.GetSESClientByName("missing"))
		assert.Nil(t, e.GetS3ClientByName("missing"))
		assert.Nil(t, e.GetMemcachedClientByName("missing"))
		assert.Nil(t, e.GetMongoDBClientByName("missing"))
		assert.Nil(t, e.GetRabbitMQClientByName("missing"))
	})

	assert.NotPanics(t, func() {
		assert.Nil(t, e.GetRepositoryConfig("missing"))
		assert.Nil(t, e.GetUseCaseConfig("missing"))
		assert.Nil(t, e.GetHandlerConfig("missing"))
		assert.Nil(t, e.GetBatchConfig("missing"))
		assert.Nil(t, e.GetCustomClient("missing"))
	})
}

func TestEngine_GettersReturnRegisteredValues(t *testing.T) {
	e := &Engine{
		Log:      &mockLogger{},
		Conf:     &viper.Config{},
		Services: NewServiceRegistry(),
		Configs:  NewConfigRegistry(),
	}
	e.Services.RESTClients = map[string]rest.Service{"api": nil}
	e.Services.SQSClients = map[string]sqs.Service{"orders": nil}
	e.Configs.Repositories = map[string]any{"users": "repo-cfg"}
	e.Configs.UseCases = map[string]any{"signup": "uc-cfg"}
	e.Configs.Handlers = map[string]any{"http": "h-cfg"}
	e.Configs.Batches = map[string]any{"nightly": "b-cfg"}

	assert.NotNil(t, e.GetLogger())
	assert.NotNil(t, e.GetConfig())
	assert.Equal(t, "repo-cfg", e.GetRepositoryConfig("users"))
	assert.Equal(t, "uc-cfg", e.GetUseCaseConfig("signup"))
	assert.Equal(t, "h-cfg", e.GetHandlerConfig("http"))
	assert.Equal(t, "b-cfg", e.GetBatchConfig("nightly"))

	_, restExists := e.Services.RESTClients["api"]
	assert.True(t, restExists)
	_, sqsExists := e.Services.SQSClients["orders"]
	assert.True(t, sqsExists)
}

// TestEngine_ConfigFallsBackToSnapshot covers Config() on the static path,
// where no dynamic watcher ever publishes a value.
func TestEngine_ConfigFallsBackToSnapshot(t *testing.T) {
	conf := &viper.Config{}
	e := &Engine{Conf: conf}

	assert.Same(t, conf, e.Config())
	assert.Nil(t, e.DynamicConfig())
}

// TestEngine_RunWithoutRouterIsAnError guards the entry point consumers call
// last; without a router there is nothing to run.
func TestEngine_RunWithoutRouterIsAnError(t *testing.T) {
	e := &Engine{ctx: context.Background()}
	require.Error(t, e.Run())
}

func TestEngine_GetContext(t *testing.T) {
	ctx := context.Background()
	e := &Engine{ctx: ctx}
	assert.Equal(t, ctx, e.GetContext())
}

// TestNewApp covers the direct constructor used by consumers that do not want
// the fluent builder.
func TestNewApp(t *testing.T) {
	app := NewApp()
	require.NotNil(t, app)
	assert.NotNil(t, app.Engine)
}
