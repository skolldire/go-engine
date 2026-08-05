package circuit_breaker

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/sony/gobreaker"
)

func NewCircuitBreaker(d Dependencies) *CircuitBreaker {
	cfg := normalizeCBConfig(d.Config)
	settings := gobreaker.Settings{
		Name:          cfg.Name,
		MaxRequests:   cfg.MaxRequests,
		Interval:      cfg.Interval,
		Timeout:       cfg.Timeout,
		ReadyToTrip:   createReadyToTripFunc(cfg, d.Log),
		OnStateChange: createOnStateChangeFunc(cfg, d.Log),
	}

	return &CircuitBreaker{
		cb:     gobreaker.NewCircuitBreaker(settings),
		config: cfg,
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
				log.Warn(context.Background(), "circuit breaker changing to open state",
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
			log.Warn(context.Background(), "circuit breaker state changed",
				map[string]interface{}{"circuit": name,
					"from": stateToString(from),
					"to":   stateToString(to)})
		}
	}
}

func stateToString(state gobreaker.State) string {
	switch state {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateHalfOpen:
		return "half-open"
	case gobreaker.StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// normalizeCBConfig returns a Config with defaults applied for any missing or
// invalid field. It never mutates the caller-supplied config and tolerates a
// nil input, in which case a fully defaulted config is returned.
func normalizeCBConfig(cfg *Config) *Config {
	out := &Config{}
	if cfg != nil {
		*out = *cfg
	}

	if out.Name == "" {
		out.Name = DefaultCBName
	}

	if out.MaxRequests == 0 {
		out.MaxRequests = DefaultCBMaxRequests
	}

	if out.Interval <= 0 {
		out.Interval = DefaultCBInterval
	}

	if out.Timeout <= 0 {
		out.Timeout = DefaultCBTimeout
	}

	if out.RequestThreshold == 0 {
		out.RequestThreshold = DefaultCBRequestThreshold
	}

	if out.FailureRateThreshold <= 0 || out.FailureRateThreshold > 1.0 {
		out.FailureRateThreshold = DefaultCBFailureRateThreshold
	}

	return out
}
