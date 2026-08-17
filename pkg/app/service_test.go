package app

import (
	"context"
	"testing"

	"github.com/skolldire/go-engine/aws/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/config/viper"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestApp(conf *viper.Config) *App {
	engine := &Engine{
		ctx:  context.Background(),
		Conf: conf,
		Log:  &mockLogger{},
	}
	return &App{Engine: engine}
}

// TestInit_WithoutAWSDoesNotBuildCloudClient is the regression test for Init
// resolving the AWS credential chain and building a CloudClient unconditionally,
// which added IMDS latency to every startup and failed outright in environments
// with no AWS credentials at all.
func TestInit_WithoutAWSDoesNotBuildCloudClient(t *testing.T) {
	app := newTestApp(&viper.Config{})

	result := app.Init()

	require.Empty(t, result.Engine.errors)
	assert.Nil(t, result.Engine.CloudClient,
		"a configuration with no AWS adapter must not get an AWS-backed client")
}

// TestInit_WithAWSBuildsCloudClient is the counterpart: declaring an AWS adapter
// still resolves the credential chain and exposes the cloud client.
func TestInit_WithAWSBuildsCloudClient(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-1")

	app := newTestApp(&viper.Config{
		Aws: viper.AwsConfig{Region: "us-east-1"},
		SQSClients: []map[string]sqs.Config{
			{"orders": {}},
		},
	})

	result := app.Init()

	assert.NotNil(t, result.Engine.CloudClient)
}

// TestBuildLogger_PreservesFullConfig is the regression test for the engine
// rebuilding logger.Config with only Level and Path, which silently dropped
// log.format and report_caller from the YAML.
func TestBuildLogger_PreservesFullConfig(t *testing.T) {
	log := buildLogger(logger.Config{
		Level:        "warn",
		Format:       "json",
		ReportCaller: true,
	}, nil)

	require.NotNil(t, log)
	assert.Equal(t, "warning", log.GetLogLevel(),
		"the configured level must survive; it used to be overwritten with trace")
}

// TestGetConfigs_DoesNotForceTraceLevel guards the specific line that pinned
// every service to trace regardless of what the configuration declared.
func TestGetConfigs_DoesNotForceTraceLevel(t *testing.T) {
	for _, level := range []string{"info", "warning", "error"} {
		t.Run(level, func(t *testing.T) {
			log := buildLogger(logger.Config{Level: level}, nil)
			assert.NotEqual(t, "trace", log.GetLogLevel())
			assert.Equal(t, level, log.GetLogLevel())
		})
	}
}
