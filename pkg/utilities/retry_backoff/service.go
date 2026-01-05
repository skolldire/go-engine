package retry_backoff

import (
	"context"
	"math"
	"math/rand"
	"time"
)

func NewRetryer(d Dependencies) *Retryer {
	validateConfig(d.RetryConfig)
	settings := &Config{
		InitialWaitTime: d.RetryConfig.InitialWaitTime * time.Millisecond,
		MaxWaitTime:     d.RetryConfig.MaxWaitTime * time.Second,
		MaxRetries:      d.RetryConfig.MaxRetries,
		BackoffFactor:   d.RetryConfig.BackoffFactor,
		JitterFactor:    d.RetryConfig.JitterFactor,
	}
	return &Retryer{
		config: settings,
		logger: d.Logger,
	}
}

func validateConfig(cfg *Config) {
	if cfg.InitialWaitTime <= 0 {
		cfg.InitialWaitTime = DefaultInitialWaitTime
	}

	if cfg.MaxWaitTime <= 0 {
		cfg.MaxWaitTime = DefaultMaxWaitTime
	}

	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultMaxRetries
	}

	if cfg.BackoffFactor <= 0 {
		cfg.BackoffFactor = DefaultBackoffFactor
	}

	if cfg.JitterFactor < 0 {
		cfg.JitterFactor = DefaultJitterFactor
	}
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

	if r.logger != nil {
		return r.logger.WrapError(err, "error executing operation after all retries")
	}
	return err
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
