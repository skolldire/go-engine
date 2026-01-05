package resilience

import (
	"context"
	"errors"

	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
	"github.com/sony/gobreaker"
)

var (
	ErrOperationFailed = errors.New("operation failed after retries and circuit breaker")
)

// NewResilienceService creates a Service configured with the provided Config and logger.
// It initializes a retryer using config.RetryConfig and a circuit breaker using config.CircuitBreakerConfig.
func NewResilienceService(config Config, log logger.Service) *Service {
	return &Service{
		retryer: retry_backoff.NewRetryer(retry_backoff.Dependencies{
			RetryConfig: config.RetryConfig,
			Logger:      log,
		}),
		circuitBreaker: circuit_breaker.NewCircuitBreaker(circuit_breaker.Dependencies{
			Config: config.CircuitBreakerConfig,
			Log:    log,
		}),
		logger: log,
	}
}

func (rs *Service) Execute(ctx context.Context,
	operation func() (interface{}, error)) (interface{}, error) {
	result, err := rs.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		var opResult interface{}

		retryErr := rs.retryer.Do(ctx, func() error {
			var err error
			opResult, err = operation()
			return err
		})

		if retryErr != nil {
			if rs.logger != nil {
				rs.logger.Error(ctx, retryErr, nil)
			}
			return nil, retryErr
		}

		return opResult, nil
	})

	if err != nil {
		if errors.Is(err, circuit_breaker.ErrCircuitOpen) {
			if rs.logger != nil {
				rs.logger.Warn(ctx, "circuit breaker open, rejecting request", nil)
			}
		}

		return nil, err
	}

	return result, nil
}

func (rs *Service) CircuitBreakerState() string {
	return rs.circuitBreaker.StateAsString()
}

func (rs *Service) IsCircuitOpen() bool {
	return rs.circuitBreaker.State() == gobreaker.StateOpen
}