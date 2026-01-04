package circuit_breaker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/sony/gobreaker"
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

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(Dependencies{
		Config: &Config{Name: "test"},
		Log:    nil,
	})
	
	assert.NotNil(t, cb)
}

func TestNewCircuitBreaker_WithDefaults(t *testing.T) {
	cb := NewCircuitBreaker(Dependencies{
		Config: &Config{}, // Empty config should use defaults
		Log:    nil,
	})
	
	assert.NotNil(t, cb)
	assert.Equal(t, DefaultCBName, cb.config.Name)
}

func TestCircuitBreaker_Execute_Success(t *testing.T) {
	cb := NewCircuitBreaker(Dependencies{
		Config: &Config{Name: "test"},
		Log:    nil,
	})
	
	result, err := cb.Execute(context.Background(), func() (interface{}, error) {
		return "success", nil
	})
	
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestCircuitBreaker_Execute_Error(t *testing.T) {
	cb := NewCircuitBreaker(Dependencies{
		Config: &Config{Name: "test"},
		Log:    nil,
	})
	
	testErr := errors.New("test error")
	result, err := cb.Execute(context.Background(), func() (interface{}, error) {
		return nil, testErr
	})
	
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestCircuitBreaker_Execute_ContextCancelled(t *testing.T) {
	cb := NewCircuitBreaker(Dependencies{
		Config: &Config{Name: "test"},
		Log:    nil,
	})
	
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	
	result, err := cb.Execute(ctx, func() (interface{}, error) {
		return "success", nil
	})
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, context.Canceled, err)
}

func TestCircuitBreaker_State(t *testing.T) {
	cb := NewCircuitBreaker(Dependencies{
		Config: &Config{Name: "test"},
		Log:    nil,
	})
	
	state := cb.State()
	assert.Equal(t, gobreaker.StateClosed, state) // Initially closed
}

func TestCircuitBreaker_StateAsString(t *testing.T) {
	cb := NewCircuitBreaker(Dependencies{
		Config: &Config{Name: "test"},
		Log:    nil,
	})
	
	stateStr := cb.StateAsString()
	assert.NotEmpty(t, stateStr)
	assert.Equal(t, "cerrado", stateStr) // Initially closed
}

func TestStateToString(t *testing.T) {
	tests := []struct {
		name     string
		state    gobreaker.State
		expected string
	}{
		{"closed", gobreaker.StateClosed, "cerrado"},
		{"half-open", gobreaker.StateHalfOpen, "semi-abierto"},
		{"open", gobreaker.StateOpen, "abierto"},
		{"unknown", gobreaker.State(999), "desconocido"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stateToString(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateCBConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		checkFn  func(*testing.T, *Config)
	}{
		{
			name: "all defaults",
			config: &Config{},
			checkFn: func(t *testing.T, cfg *Config) {
				assert.Equal(t, DefaultCBName, cfg.Name)
				assert.Equal(t, uint32(DefaultCBMaxRequests), cfg.MaxRequests)
				assert.Equal(t, uint32(DefaultCBRequestThreshold), cfg.RequestThreshold)
			},
		},
		{
			name: "valid config",
			config: &Config{
				Name:                 "custom",
				MaxRequests:          200,
				Interval:             30 * time.Second,
				Timeout:              15 * time.Second,
				RequestThreshold:     10,
				FailureRateThreshold: 0.6,
			},
			checkFn: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "custom", cfg.Name)
				assert.Equal(t, uint32(200), cfg.MaxRequests)
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validateCBConfig(tt.config)
			tt.checkFn(t, tt.config)
		})
	}
}

