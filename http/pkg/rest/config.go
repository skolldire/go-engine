package rest

import "time"

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
