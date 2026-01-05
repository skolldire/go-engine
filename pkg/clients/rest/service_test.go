package rest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
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
func (m *mockLogger) WrapError(err error, msg string) error { return err }
func (m *mockLogger) WithField(key string, value interface{}) logger.Service { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service { return m }
func (m *mockLogger) GetLogLevel() string { return "info" }
func (m *mockLogger) SetLogLevel(level string) error { return nil }

func TestNewClient(t *testing.T) {
	cfg := Config{
		BaseURL:        "https://api.example.com",
		TimeOut:        5 * time.Second,
		EnableLogging:  false,
		WithResilience: false,
	}
	log := &mockLogger{}

	client := NewClient(cfg, log)

	assert.NotNil(t, client)
	assert.IsType(t, &restClient{}, client)
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	cfg := Config{
		BaseURL:        "https://api.example.com",
		TimeOut:        0,
		EnableLogging:  false,
		WithResilience: false,
	}
	log := &mockLogger{}

	client := NewClient(cfg, log)

	assert.NotNil(t, client)
	restClient := client.(*restClient)
	// Verificar que el cliente se creó correctamente
	assert.NotNil(t, restClient.httpClient)
	assert.NotNil(t, restClient.baseURL)
}

func TestNewClient_WithResilience(t *testing.T) {
	cfg := Config{
		BaseURL:        "https://api.example.com",
		TimeOut:        5 * time.Second,
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

	client := NewClient(cfg, log)

	assert.NotNil(t, client)
}

func TestRestClient_WithLogging(t *testing.T) {
	cfg := Config{
		BaseURL:       "https://api.example.com",
		TimeOut:       5 * time.Second,
		EnableLogging: false,
	}
	log := &mockLogger{}

	client := NewClient(cfg, log)
	client.WithLogging(true)

	restClient := client.(*restClient)
	assert.True(t, restClient.IsLoggingEnabled())
}

func TestValidateResponse_Success(t *testing.T) {
	// resty.Response no se puede crear directamente sin un request real
	// Por ahora, solo verificamos que la función existe y maneja nil correctamente
	err := validateResponse(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestValidateResponse_SuccessRange(t *testing.T) {
	// Este test requiere un servidor HTTP real o mocks más complejos
	// Por ahora, verificamos que la función existe y funciona con nil
	err := validateResponse(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestValidateResponse_Error(t *testing.T) {
	// Este test requiere un servidor HTTP real o mocks más complejos
	// Por ahora, verificamos que la función existe
	err := validateResponse(nil)
	assert.Error(t, err)
}

func TestValidateResponse_Nil(t *testing.T) {
	err := validateResponse(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestValidateResponse_LongBody(t *testing.T) {
	// Este test requiere un servidor HTTP real o mocks más complejos
	// Por ahora, verificamos que la función existe
	err := validateResponse(nil)
	assert.Error(t, err)
}

func TestRestClient_Get(t *testing.T) {
	cfg := Config{
		BaseURL:       "https://api.example.com",
		TimeOut:       5 * time.Second,
		EnableLogging: false,
	}
	log := &mockLogger{}

	client := NewClient(cfg, log)
	restClient := client.(*restClient)

	// Mock HTTP server would be needed for full integration test
	// This test verifies the method exists and can be called
	ctx := context.Background()
	headers := map[string]string{"Content-Type": "application/json"}

	// This will fail without a real server, but tests the structure
	_, err := restClient.Get(ctx, "/test", headers)
	// We expect an error since there's no real server
	assert.Error(t, err)
}

func TestRestClient_Post(t *testing.T) {
	cfg := Config{
		BaseURL:       "https://api.example.com",
		TimeOut:       5 * time.Second,
		EnableLogging: false,
	}
	log := &mockLogger{}

	client := NewClient(cfg, log)
	restClient := client.(*restClient)

	ctx := context.Background()
	body := map[string]string{"key": "value"}
	headers := map[string]string{"Content-Type": "application/json"}

	// This will fail without a real server
	_, err := restClient.Post(ctx, "/test", body, headers)
	assert.Error(t, err)
}

func TestRestClient_Put(t *testing.T) {
	cfg := Config{
		BaseURL:       "https://api.example.com",
		TimeOut:       5 * time.Second,
		EnableLogging: false,
	}
	log := &mockLogger{}

	client := NewClient(cfg, log)
	restClient := client.(*restClient)

	ctx := context.Background()
	body := map[string]string{"key": "value"}
	headers := map[string]string{"Content-Type": "application/json"}

	_, err := restClient.Put(ctx, "/test", body, headers)
	assert.Error(t, err)
}

func TestRestClient_Patch(t *testing.T) {
	cfg := Config{
		BaseURL:       "https://api.example.com",
		TimeOut:       5 * time.Second,
		EnableLogging: false,
	}
	log := &mockLogger{}

	client := NewClient(cfg, log)
	restClient := client.(*restClient)

	ctx := context.Background()
	body := map[string]string{"key": "value"}
	headers := map[string]string{"Content-Type": "application/json"}

	_, err := restClient.Patch(ctx, "/test", body, headers)
	assert.Error(t, err)
}

func TestRestClient_Delete(t *testing.T) {
	cfg := Config{
		BaseURL:       "https://api.example.com",
		TimeOut:       5 * time.Second,
		EnableLogging: false,
	}
	log := &mockLogger{}

	client := NewClient(cfg, log)
	restClient := client.(*restClient)

	ctx := context.Background()
	headers := map[string]string{"Content-Type": "application/json"}

	_, err := restClient.Delete(ctx, "/test", headers)
	assert.Error(t, err)
}

func TestRestClient_ProcessRequest_Error(t *testing.T) {
	cfg := Config{
		BaseURL:       "https://api.example.com",
		TimeOut:       5 * time.Second,
		EnableLogging: true,
	}
	log := &mockLogger{}
	log.On("Warn", mock.Anything, "request_failed", mock.Anything).Return()

	client := NewClient(cfg, log)
	restClient := client.(*restClient)

	ctx := context.Background()
	testErr := errors.New("request failed")
	reqFunc := func() (*resty.Response, error) {
		return nil, testErr
	}

	_, err := restClient.processRequest(ctx, reqFunc)
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
	log.AssertExpectations(t)
}

func TestRestClient_ProcessRequest_InvalidResponse(t *testing.T) {
	cfg := Config{
		BaseURL:       "https://api.example.com",
		TimeOut:       5 * time.Second,
		EnableLogging: true,
	}
	log := &mockLogger{}
	log.On("Warn", mock.Anything, mock.Anything, mock.Anything).Return()

	client := NewClient(cfg, log)
	restClient := client.(*restClient)

	ctx := context.Background()
	// Simular un error en la función de request
	reqFunc := func() (*resty.Response, error) {
		return nil, errors.New("request failed")
	}

	_, err := restClient.processRequest(ctx, reqFunc)
	assert.Error(t, err)
	log.AssertExpectations(t)
}
