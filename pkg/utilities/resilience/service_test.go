package resilience

import (
	"context"
	"errors"
	"testing"

	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
	"github.com/stretchr/testify/assert"
)

func TestNewResilienceService(t *testing.T) {
	config := Config{
		RetryConfig: &retry_backoff.Config{
			MaxRetries: 3,
		},
		CircuitBreakerConfig: &circuit_breaker.Config{
			Name: "test",
		},
	}

	service := NewResilienceService(config, nil)
	assert.NotNil(t, service)
}

func TestService_Execute_Success(t *testing.T) {
	config := Config{
		RetryConfig: &retry_backoff.Config{
			MaxRetries: 1,
		},
		CircuitBreakerConfig: &circuit_breaker.Config{
			Name: "test",
		},
	}

	service := NewResilienceService(config, nil)

	result, err := service.Execute(context.Background(), func() (interface{}, error) {
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestService_Execute_Error(t *testing.T) {
	config := Config{
		RetryConfig: &retry_backoff.Config{
			MaxRetries: 1,
		},
		CircuitBreakerConfig: &circuit_breaker.Config{
			Name: "test",
		},
	}

	service := NewResilienceService(config, nil)
	testErr := errors.New("test error")

	result, err := service.Execute(context.Background(), func() (interface{}, error) {
		return nil, testErr
	})

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestService_CircuitBreakerState(t *testing.T) {
	config := Config{
		RetryConfig: &retry_backoff.Config{
			MaxRetries: 1,
		},
		CircuitBreakerConfig: &circuit_breaker.Config{
			Name: "test",
		},
	}

	service := NewResilienceService(config, nil)
	state := service.CircuitBreakerState()
	assert.NotEmpty(t, state)
}

func TestService_IsCircuitOpen(t *testing.T) {
	config := Config{
		RetryConfig: &retry_backoff.Config{
			MaxRetries: 1,
		},
		CircuitBreakerConfig: &circuit_breaker.Config{
			Name: "test",
		},
	}

	service := NewResilienceService(config, nil)
	isOpen := service.IsCircuitOpen()
	assert.False(t, isOpen) // Initially closed
}
