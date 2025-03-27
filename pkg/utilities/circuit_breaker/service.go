package circuit_breaker

import (
	"context"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/sony/gobreaker"
)

func NewCircuitBreaker(d Dependencies) *CircuitBreaker {
	validateCBConfig(d.Config)
	settings := gobreaker.Settings{
		Name:          d.Config.Name,
		MaxRequests:   d.Config.MaxRequests,
		Interval:      d.Config.Interval * time.Second,
		Timeout:       d.Config.Timeout * time.Second,
		ReadyToTrip:   createReadyToTripFunc(d.Config, d.Log),
		OnStateChange: createOnStateChangeFunc(d.Config, d.Log),
	}

	return &CircuitBreaker{
		cb:     gobreaker.NewCircuitBreaker(settings),
		config: d.Config,
		log:    d.Log,
	}
}

func (cb *CircuitBreaker) Execute(ctx context.Context, operation func() (interface{}, error)) (interface{}, error) {
	result, err := cb.cb.Execute(func() (interface{}, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return operation()
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return nil, ErrCircuitOpen
		}
		if err == gobreaker.ErrTooManyRequests {
			return nil, ErrTooManyCalls
		}
		return nil, err
	}

	return result, nil
}

func (cb *CircuitBreaker) State() gobreaker.State {
	return cb.cb.State()
}

func (cb *CircuitBreaker) StateAsString() string {
	return stateToString(cb.cb.State())
}

func createReadyToTripFunc(config *Config, log logger.Service) func(counts gobreaker.Counts) bool {
	return func(counts gobreaker.Counts) bool {
		if counts.Requests >= config.RequestThreshold {
			failureRate := float64(counts.TotalFailures) / float64(counts.Requests)
			shouldTrip := failureRate >= config.FailureRateThreshold

			if shouldTrip && log != nil {
				log.Warn(context.Background(), "Circuit breaker cambiando a estado abierto",
					map[string]interface{}{"circuit": config.Name,
						"requests":    counts.Requests,
						"failures":    counts.TotalFailures,
						"failureRate": failureRate,
						"threshold":   config.FailureRateThreshold})
			}

			return shouldTrip
		}
		return false
	}
}

func createOnStateChangeFunc(config *Config, log logger.Service) func(name string, from gobreaker.State, to gobreaker.State) {
	return func(name string, from gobreaker.State, to gobreaker.State) {
		if log != nil {
			log.Warn(context.Background(), "Circuit breaker estado cambiado",
				map[string]interface{}{"circuit": name,
					"from": stateToString(from),
					"to":   stateToString(to)})
		}
	}
}

func stateToString(state gobreaker.State) string {
	switch state {
	case gobreaker.StateClosed:
		return "cerrado"
	case gobreaker.StateHalfOpen:
		return "semi-abierto"
	case gobreaker.StateOpen:
		return "abierto"
	default:
		return "desconocido"
	}
}

func validateCBConfig(cfg *Config) {
	if cfg.Name == "" {
		cfg.Name = DefaultCBName
	}

	if cfg.MaxRequests == 0 {
		cfg.MaxRequests = DefaultCBMaxRequests
	}

	if cfg.Interval <= 0 {
		cfg.Interval = DefaultCBInterval
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultCBTimeout
	}

	if cfg.RequestThreshold == 0 {
		cfg.RequestThreshold = DefaultCBRequestThreshold
	}

	if cfg.FailureRateThreshold <= 0 || cfg.FailureRateThreshold > 1.0 {
		cfg.FailureRateThreshold = DefaultCBFailureRateThreshold
	}
}
