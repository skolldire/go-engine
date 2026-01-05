package aws

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/skolldire/go-engine/pkg/integration/observability"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/otel/attribute"
)

// mockClient is a mock implementation of cloud.Client
type mockClient struct {
	mock.Mock
}

func (m *mockClient) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cloud.Response), args.Error(1)
}

func TestNew(t *testing.T) {
	cfg := aws.Config{
		Region: "us-east-1",
	}

	client := New(cfg)
	assert.NotNil(t, client)
}

func TestNewWithOptions_DefaultTimeout(t *testing.T) {
	cfg := aws.Config{
		Region: "us-east-1",
	}

	client := NewWithOptions(cfg, Options{})
	assert.NotNil(t, client)

	// Test that default timeout is applied
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "test-queue",
	}

	// This will fail because we don't have a real AWS connection, but it tests the structure
	_, err := client.Do(context.Background(), req)
	// We expect an error because we don't have real AWS credentials
	assert.Error(t, err)
}

func TestNewWithOptions_CustomTimeout(t *testing.T) {
	cfg := aws.Config{
		Region: "us-east-1",
	}

	client := NewWithOptions(cfg, Options{
		Timeout: 10 * time.Second,
	})
	assert.NotNil(t, client)
}

func TestNewWithOptions_WithRetry(t *testing.T) {
	cfg := aws.Config{
		Region: "us-east-1",
	}

	opts := WithRetry()
	client := NewWithOptions(cfg, opts)
	assert.NotNil(t, client)

	// Verify retry policy is set
	assert.True(t, opts.RetryPolicy.Enabled)
	assert.Equal(t, 3, opts.RetryPolicy.MaxAttempts)
	assert.Contains(t, opts.RetryPolicy.RetriableErrors, cloud.ErrCodeThrottling)
}

func TestNewWithOptions_WithObservability(t *testing.T) {
	cfg := aws.Config{
		Region: "us-east-1",
	}

	// Create mock logger, metrics recorder, and tracer
	mockLogger := &mockLogger{}
	mockMetricsRecorder := &mockMetricsRecorder{}
	mockTracer := &mockTracer{}

	opts := WithObservability(mockLogger, mockMetricsRecorder, mockTracer)
	client := NewWithOptions(cfg, opts)
	assert.NotNil(t, client)

	// Verify middlewares are set
	assert.Len(t, opts.Middlewares, 3)
}

func TestNewWithOptions_WithObservability_NilValues(t *testing.T) {
	cfg := aws.Config{
		Region: "us-east-1",
	}

	opts := WithObservability(nil, nil, nil)
	client := NewWithOptions(cfg, opts)
	assert.NotNil(t, client)

	// Verify no middlewares are set when all are nil
	assert.Len(t, opts.Middlewares, 0)
}

func TestNewWithOptions_WithObservability_PartialNil(t *testing.T) {
	cfg := aws.Config{
		Region: "us-east-1",
	}

	mockLogger := &mockLogger{}
	mockMetricsRecorder := &mockMetricsRecorder{}

	opts := WithObservability(mockLogger, mockMetricsRecorder, nil)
	client := NewWithOptions(cfg, opts)
	assert.NotNil(t, client)

	// Verify only 2 middlewares are set (logging and metrics)
	assert.Len(t, opts.Middlewares, 2)
}

func TestRetryPolicy_DefaultValues(t *testing.T) {
	policy := RetryPolicy{}
	assert.False(t, policy.Enabled)
	assert.Equal(t, 0, policy.MaxAttempts)
	assert.Nil(t, policy.RetriableErrors)
}

// Mock implementations for testing
type mockLogger struct{}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{})     {}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]interface{})      {}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]interface{})      {}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]interface{})      {}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {}
func (m *mockLogger) WrapError(err error, msg string) error                                    { return err }
func (m *mockLogger) WithField(key string, value interface{}) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                                      { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                           { return nil }

type mockMetricsRecorder struct{}

func (m *mockMetricsRecorder) RecordRequest(operation string, duration time.Duration, statusCode int, errorCode string) {
}
func (m *mockMetricsRecorder) RecordRetry(operation string)    {}
func (m *mockMetricsRecorder) RecordThrottle(operation string) {}

// Ensure mockMetricsRecorder implements observability.MetricsRecorder
var _ observability.MetricsRecorder = (*mockMetricsRecorder)(nil)

type mockTracer struct{}

func (m *mockTracer) Span(ctx context.Context, name string, fn func(context.Context) error, attrs ...attribute.KeyValue) error {
	return fn(ctx)
}
