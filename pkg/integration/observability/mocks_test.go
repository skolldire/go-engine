package observability

import (
	"context"
	"time"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"go.opentelemetry.io/otel/attribute"
	"github.com/stretchr/testify/mock"
)

// mockLogger is a mock implementation of logger.Service
type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}

func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}

func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}

func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]interface{}) {
	m.Called(ctx, err, fields)
}

func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {
	m.Called(ctx, err, fields)
}

func (m *mockLogger) WrapError(err error, msg string) error {
	args := m.Called(err, msg)
	return args.Error(0)
}

func (m *mockLogger) WithField(key string, value interface{}) logger.Service {
	args := m.Called(key, value)
	return args.Get(0).(logger.Service)
}

func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service {
	args := m.Called(fields)
	return args.Get(0).(logger.Service)
}

func (m *mockLogger) GetLogLevel() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockLogger) SetLogLevel(level string) error {
	args := m.Called(level)
	return args.Error(0)
}

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

// mockMetricsRecorder is a mock implementation of MetricsRecorder
type mockMetricsRecorder struct {
	mock.Mock
}

func (m *mockMetricsRecorder) RecordRequest(operation string, duration time.Duration, statusCode int, errorCode string) {
	m.Called(operation, duration, statusCode, errorCode)
}

func (m *mockMetricsRecorder) RecordRetry(operation string) {
	m.Called(operation)
}

func (m *mockMetricsRecorder) RecordThrottle(operation string) {
	m.Called(operation)
}

// mockTracer is a mock implementation of telemetry.Tracer
type mockTracer struct {
	mock.Mock
}

func (m *mockTracer) Span(ctx context.Context, name string, fn func(context.Context) error, attrs ...attribute.KeyValue) error {
	args := m.Called(ctx, name, fn, attrs)
	if args.Error(0) != nil {
		return args.Error(0)
	}
	return fn(ctx)
}

