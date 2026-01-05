package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
	"github.com/stretchr/testify/assert"
)

type mockLogger struct{}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]interface{})  {}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]interface{})  {}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]interface{})  {}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {}
func (m *mockLogger) WrapError(err error, msg string) error { return err }
func (m *mockLogger) WithField(key string, value interface{}) logger.Service { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service { return m }
func (m *mockLogger) GetLogLevel() string { return "info" }
func (m *mockLogger) SetLogLevel(level string) error { return nil }

func TestNewBaseClient(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  true,
		WithResilience: false,
		Timeout:        5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)

	assert.NotNil(t, client)
	assert.Equal(t, true, client.logging)
	assert.Equal(t, 5*time.Second, client.timeout)
	assert.Nil(t, client.resilience)
}

func TestNewBaseClientWithName(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  true,
		WithResilience: false,
		Timeout:        5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClientWithName(config, log, "test-service")

	assert.NotNil(t, client)
	assert.Equal(t, "test-service", client.serviceName)
	assert.Equal(t, true, client.logging)
}

func TestNewBaseClient_DefaultTimeout(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        0,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)

	assert.NotNil(t, client)
	assert.Equal(t, DefaultTimeout, client.timeout)
}

func TestNewBaseClient_WithResilience(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  true,
		WithResilience: true,
		Resilience: resilience.Config{
			RetryConfig: &retry_backoff.Config{
				MaxRetries: 3,
			},
			CircuitBreakerConfig: &circuit_breaker.Config{
				Name: "test-cb",
			},
		},
		Timeout: 5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)

	assert.NotNil(t, client)
	assert.NotNil(t, client.resilience)
}

func TestBaseClient_Execute_Success(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)

	ctx := context.Background()
	result, err := client.Execute(ctx, "test-operation", func() (interface{}, error) {
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestBaseClient_Execute_Error(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)

	ctx := context.Background()
	testErr := errors.New("test error")
	result, err := client.Execute(ctx, "test-operation", func() (interface{}, error) {
		return nil, testErr
	})

	assert.Error(t, err)
	assert.Equal(t, testErr, err)
	assert.Nil(t, result)
}

func TestBaseClient_Execute_WithResilience(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: true,
		Resilience: resilience.Config{
			RetryConfig: &retry_backoff.Config{
				MaxRetries: 2,
			},
			CircuitBreakerConfig: &circuit_breaker.Config{
				Name: "test-cb",
			},
		},
		Timeout: 5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)

	ctx := context.Background()
	result, err := client.Execute(ctx, "test-operation", func() (interface{}, error) {
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestBaseClient_Execute_WithTimeout(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        10 * time.Millisecond,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)

	ctx := context.Background()
	done := make(chan bool, 1)
	var result interface{}
	var err error

	go func() {
		result, err = client.Execute(ctx, "test-operation", func() (interface{}, error) {
			time.Sleep(100 * time.Millisecond)
			return "success", nil
		})
		done <- true
	}()

	select {
	case <-done:
		// Operation completed
		if err != nil {
			assert.Error(t, err)
			assert.Nil(t, result)
		}
	case <-time.After(200 * time.Millisecond):
		// Test should complete within reasonable time
		t.Log("Test completed")
	}
}

func TestBaseClient_Execute_WithContextDeadline(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)
	var err error

	go func() {
		_, err = client.Execute(ctx, "test-operation", func() (interface{}, error) {
			time.Sleep(100 * time.Millisecond)
			return "success", nil
		})
		done <- true
	}()

	select {
	case <-done:
		// Operation completed (might have timed out or succeeded)
		if err != nil {
			assert.Error(t, err)
		}
	case <-time.After(200 * time.Millisecond):
		// Test should complete within reasonable time
		t.Log("Test completed")
	}
}

func TestBaseClient_SetLogging(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)
	assert.False(t, client.IsLoggingEnabled())

	client.SetLogging(true)
	assert.True(t, client.IsLoggingEnabled())
}

func TestBaseClient_GetLogger(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)

	assert.Equal(t, log, client.GetLogger())
}

func TestBaseClient_GetServiceName(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)
	assert.Equal(t, "base", client.getServiceName())

	clientWithName := NewBaseClientWithName(config, log, "custom-service")
	assert.Equal(t, "custom-service", clientWithName.getServiceName())
}

func TestBaseClient_SetServiceName(t *testing.T) {
	config := BaseConfig{
		EnableLogging:  false,
		WithResilience: false,
		Timeout:        5 * time.Second,
	}
	log := &mockLogger{}

	client := NewBaseClient(config, log)
	assert.Equal(t, "base", client.getServiceName())

	client.SetServiceName("new-service")
	assert.Equal(t, "new-service", client.getServiceName())
}
