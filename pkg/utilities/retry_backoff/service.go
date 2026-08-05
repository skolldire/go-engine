package retry_backoff

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

func NewRetryer(d Dependencies) *Retryer {
	settings := normalizeConfig(d.RetryConfig)
	return &Retryer{
		config: settings,
		logger: d.Logger,
	}
}

// normalizeConfig returns a Config with defaults applied for any missing or
// invalid field. It never mutates the caller-supplied config and tolerates a
// nil input, in which case a fully defaulted config is returned.
func normalizeConfig(cfg *Config) *Config {
	out := &Config{}
	if cfg != nil {
		*out = *cfg
	}

	if out.InitialWaitTime <= 0 {
		out.InitialWaitTime = DefaultInitialWaitTime
	}

	if out.MaxWaitTime <= 0 {
		out.MaxWaitTime = DefaultMaxWaitTime
	}

	if out.MaxRetries <= 0 {
		out.MaxRetries = DefaultMaxRetries
	}

	if out.BackoffFactor <= 0 {
		out.BackoffFactor = DefaultBackoffFactor
	}

	if out.JitterFactor < 0 {
		out.JitterFactor = DefaultJitterFactor
	}

	return out
}

func (r *Retryer) Do(ctx context.Context, operation func() error) error {
	var err error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		err = operation()

		if err == nil || ctx.Err() != nil {
			return err
		}

		if attempt == r.config.MaxRetries {
			return err
		}

		waitTime := r.calculateWaitTime(attempt)

		if r.logger != nil {
			r.logger.Debug(ctx, "retrying operation after error",
				map[string]interface{}{"attempt": attempt + 1,
					"maxRetries": r.config.MaxRetries,
					"waitTime":   waitTime,
					"error":      err.Error()})
		}

		select {
		case <-time.After(waitTime):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Always wrap error with consistent message regardless of logger configuration
	wrappedErr := fmt.Errorf("error executing operation after all retries: %w", err)
	if r.logger != nil {
		// Log the error if logger is available
		r.logger.Error(ctx, wrappedErr, nil)
	}
	return wrappedErr
}

func (r *Retryer) calculateWaitTime(attempt int) time.Duration {
	baseWaitTime := r.config.InitialWaitTime * time.Duration(math.Pow(r.config.BackoffFactor, float64(attempt)))

	jitter := time.Duration(rand.Float64() * r.config.JitterFactor * float64(baseWaitTime))
	waitTime := baseWaitTime + jitter

	if waitTime > r.config.MaxWaitTime {
		waitTime = r.config.MaxWaitTime
	}

	return waitTime
}
