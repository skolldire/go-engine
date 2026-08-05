package circuit_breaker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
)

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

	result, err := cb.Execute(context.Background(), func() (any, error) {
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
	result, err := cb.Execute(context.Background(), func() (any, error) {
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

	result, err := cb.Execute(ctx, func() (any, error) {
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
	assert.Equal(t, "closed", stateStr) // Initially closed
}

func TestStateToString(t *testing.T) {
	tests := []struct {
		name     string
		state    gobreaker.State
		expected string
	}{
		{"closed", gobreaker.StateClosed, "closed"},
		{"half-open", gobreaker.StateHalfOpen, "half-open"},
		{"open", gobreaker.StateOpen, "open"},
		{"unknown", gobreaker.State(999), "unknown"},
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
		name    string
		config  *Config
		checkFn func(*testing.T, *Config)
	}{
		{
			name:   "all defaults",
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
			tt.checkFn(t, normalizeCBConfig(tt.config))
		})
	}
}

func TestNormalizeCBConfigNilIsSafe(t *testing.T) {
	cfg := normalizeCBConfig(nil)
	assert.Equal(t, DefaultCBName, cfg.Name)
	assert.Equal(t, DefaultCBInterval, cfg.Interval)
	assert.Equal(t, DefaultCBTimeout, cfg.Timeout)
}

func TestNormalizeCBConfigDoesNotMutateInput(t *testing.T) {
	in := &Config{}
	_ = normalizeCBConfig(in)
	assert.Equal(t, "", in.Name, "input config must not be mutated")
	assert.Equal(t, time.Duration(0), in.Interval, "input config must not be mutated")
}
