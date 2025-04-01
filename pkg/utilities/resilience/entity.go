package resilience

import (
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
)

type Config struct {
	RetryConfig          *retry_backoff.Config   `mapstructure:"retry_config" json:"retry_config"`
	CircuitBreakerConfig *circuit_breaker.Config `mapstructure:"circuit_breaker_config" json:"circuit_breaker_config"`
}

type Service struct {
	retryer        *retry_backoff.Retryer
	circuitBreaker *circuit_breaker.CircuitBreaker
	logger         logger.Service
}
