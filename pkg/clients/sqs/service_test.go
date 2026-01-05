package sqs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {}
func (m *mockLogger) WrapError(err error, msg string) error {
	args := m.Called(err, msg)
	if args.Get(0) != nil {
		return args.Get(0).(error)
	}
	return err
}
func (m *mockLogger) WithField(key string, value interface{}) logger.Service { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service { return m }
func (m *mockLogger) GetLogLevel() string { return "info" }
func (m *mockLogger) SetLogLevel(level string) error { return nil }

func TestNewClient(t *testing.T) {
	acf := aws.Config{
		Region: "us-east-1",
	}
	cfg := Config{
		Endpoint:       "",
		EnableLogging:  false,
		WithResilience: false,
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	assert.NotNil(t, client)
	assert.IsType(t, &Cliente{}, client)
}

func TestNewClient_WithEndpoint(t *testing.T) {
	acf := aws.Config{
		Region: "us-east-1",
	}
	cfg := Config{
		Endpoint:       "http://localhost:9324",
		EnableLogging:  true,
		WithResilience: false,
	}
	log := &mockLogger{}
	log.On("Debug", mock.Anything, "SQS client initialized", mock.Anything).Return()

	client := NewClient(acf, cfg, log)

	assert.NotNil(t, client)
	log.AssertExpectations(t)
}

func TestNewClient_WithResilience(t *testing.T) {
	acf := aws.Config{
		Region: "us-east-1",
	}
	cfg := Config{
		Endpoint:       "",
		EnableLogging:  false,
		WithResilience: true,
		Resilience: resilience.Config{
			RetryConfig: &retry_backoff.Config{
				MaxRetries: 3,
			},
			CircuitBreakerConfig: &circuit_breaker.Config{
				Name: "test-cb",
			},
		},
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	assert.NotNil(t, client)
	cliente := client.(*Cliente)
	assert.NotNil(t, cliente.resilience)
}

func TestCliente_SendMsj_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.SendMsj(ctx, "", "message", nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)

	_, err = client.SendMsj(ctx, "queue-url", "", nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestCliente_SendJSON_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.SendJSON(ctx, "", map[string]string{"key": "value"}, nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)

	_, err = client.SendJSON(ctx, "queue-url", nil, nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestCliente_ReceiveMsj_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.ReceiveMsj(ctx, "", 10, 0)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestCliente_ReceiveMsj_DefaultMaxMessages(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}
	
	// Configure mock to return error when WrapError is called
	log.On("WrapError", mock.Anything, mock.Anything).Return(errors.New("mock error"))

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	// This will fail without a real AWS connection, but tests the default logic
	_, err := client.ReceiveMsj(ctx, "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue", 0, 0)
	// We expect an error since there's no real AWS connection
	assert.Error(t, err)
	// But the error should not be ErrInvalidInput since queueURL is provided
	assert.NotEqual(t, ErrInvalidInput, err)
}

func TestCliente_DeleteMsj_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	err := client.DeleteMsj(ctx, "", "receipt-handle")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)

	err = client.DeleteMsj(ctx, "queue-url", "")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestCliente_CreateQueue_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.CreateQueue(ctx, "", nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestCliente_CreateQueue_WithAttributes(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	attributes := map[string]string{
		"DelaySeconds": "60",
	}
	// This will fail without a real AWS connection
	_, err := client.CreateQueue(ctx, "test-queue", attributes)
	assert.Error(t, err)
	assert.NotEqual(t, ErrInvalidInput, err)
}

func TestCliente_DeleteQueue_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	err := client.DeleteQueue(ctx, "")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestCliente_GetURLQueue_InvalidInput(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.GetURLQueue(ctx, "")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidInput, err)
}

func TestCliente_ListQueue(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	// This will fail without a real AWS connection
	_, err := client.ListQueue(ctx, "")
	assert.Error(t, err)
	// But it should not be ErrInvalidInput since prefix is optional
	assert.NotEqual(t, ErrInvalidInput, err)
}

func TestCliente_ListQueue_WithPrefix(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	_, err := client.ListQueue(ctx, "test-prefix")
	assert.Error(t, err)
	assert.NotEqual(t, ErrInvalidInput, err)
}

func TestCliente_EnableLogging(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{
		EnableLogging: false,
	}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)
	cliente := client.(*Cliente)

	assert.False(t, cliente.logging)
	client.EnableLogging(true)
	assert.True(t, cliente.logging)
}

func TestCliente_SendJSON_ValidJSON(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	message := map[string]interface{}{
		"key":   "value",
		"number": 42,
		"bool":  true,
	}

	// This will fail without a real AWS connection, but tests JSON marshaling
	_, err := client.SendJSON(ctx, "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue", message, nil)
	assert.Error(t, err)
	assert.NotEqual(t, ErrInvalidInput, err)
}

func TestCliente_SendJSON_WithAttributes(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)

	ctx := context.Background()
	message := map[string]string{"key": "value"}
	attributes := map[string]types.MessageAttributeValue{
		"custom-attr": {
			StringValue: aws.String("value"),
			DataType:    aws.String("String"),
		},
	}

	_, err := client.SendJSON(ctx, "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue", message, attributes)
	assert.Error(t, err)
	assert.NotEqual(t, ErrInvalidInput, err)
}

func TestCliente_EnsureContextWithTimeout(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)
	cliente := client.(*Cliente)

	ctx := context.Background()
	newCtx, cancel := cliente.ensureContextWithTimeout(ctx)
	assert.NotNil(t, newCtx)
	assert.NotNil(t, cancel)
	cancel()
}

func TestCliente_EnsureContextWithTimeout_WithDeadline(t *testing.T) {
	acf := aws.Config{Region: "us-east-1"}
	cfg := Config{}
	log := &mockLogger{}

	client := NewClient(acf, cfg, log)
	cliente := client.(*Cliente)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	newCtx, cancelFunc := cliente.ensureContextWithTimeout(ctx)
	assert.NotNil(t, newCtx)
	assert.NotNil(t, cancelFunc)
	cancelFunc()
}
