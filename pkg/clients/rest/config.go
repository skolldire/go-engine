package rest

import (
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
)

// resolveResilience turns the configuration into the two decorators that may be
// applied, keeping the deprecated WithResilience flag working.
//
// Returning nil for either one means that decorator is simply not installed,
// which is the difference between composing behaviour in and shipping a feature
// that is always in the call path behind a boolean.
func (c Config) resolveResilience() (*RetryConfig, *circuit_breaker.Config) {
	retry := c.Retry
	breaker := c.CircuitBreaker

	//nolint:staticcheck // SA1019: the deprecated flag is still honoured on purpose
	if c.WithResilience {
		if retry == nil {
			retry = &RetryConfig{}
			if c.Resilience.RetryConfig != nil {
				retry.MaxRetries = c.Resilience.RetryConfig.MaxRetries
				retry.WaitTime = c.Resilience.RetryConfig.InitialWaitTime
				retry.MaxWaitTime = c.Resilience.RetryConfig.MaxWaitTime
			}
		}
		if breaker == nil {
			breaker = c.Resilience.CircuitBreakerConfig
			if breaker == nil {
				breaker = &circuit_breaker.Config{}
			}
		}
	}

	return retry, breaker
}

func (r *RetryConfig) maxRetries() int {
	if r.MaxRetries <= 0 {
		return DefaultRetryCount
	}
	return r.MaxRetries
}

func (r *RetryConfig) waitTime() time.Duration {
	if r.WaitTime <= 0 {
		return DefaultRetryWaitTime
	}
	return r.WaitTime
}

func (r *RetryConfig) maxWaitTime() time.Duration {
	if r.MaxWaitTime <= 0 {
		return DefaultRetryMaxWaitTime
	}
	return r.MaxWaitTime
}
