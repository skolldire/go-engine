package rest

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

// Resty already implements the retry engine: the loop, exponential backoff with
// jitter, and the cap on Retry-After. What it does not ship is a *policy* —
// RetryConditions are whatever the caller adds, and by default nothing is
// retried. The two functions below supply that policy and nothing else.

// RetryableStatus reports whether an HTTP status is worth retrying.
//
// 429 is the server asking us to slow down, and the listed 5xx describe a
// transient server-side condition. Everything else is permanent: retrying a 400
// or a 404 cannot succeed, it only repeats the same failure later while adding
// load to a service that may already be struggling. 501 and 505 are excluded
// deliberately — they describe a request the server will never accept.
func RetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusInsufficientStorage:
		return true
	default:
		return false
	}
}

// IsIdempotent reports whether replaying a request with this method is safe,
// per RFC 9110. POST and PATCH are not: replaying one can duplicate a side
// effect the server already applied — a second charge, a second order.
func IsIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut,
		http.MethodDelete, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// ConservativeRetryCondition is the resty retry condition applied by default.
//
// It replays only idempotent methods, and only on a transient outcome: a
// retryable status, or a transport error with no response at all.
func ConservativeRetryCondition(resp *resty.Response, err error) bool {
	if resp == nil || resp.Request == nil {
		// No response means the request never completed (connection reset, DNS,
		// timeout). Those are the canonical retryable case, but we still cannot
		// tell whether the server processed it, so only idempotent methods qualify.
		return false
	}

	if !IsIdempotent(resp.Request.Method) {
		return false
	}

	if err != nil {
		return true
	}
	return RetryableStatus(resp.StatusCode())
}

// AllowNonIdempotentRetryCondition is the opt-in variant for endpoints that are
// safe to repeat, typically because they are protected by an idempotency key.
func AllowNonIdempotentRetryCondition(resp *resty.Response, err error) bool {
	if resp == nil || resp.Request == nil {
		return false
	}
	if err != nil {
		return true
	}
	return RetryableStatus(resp.StatusCode())
}

// RetryAfter parses the Retry-After header and returns how long to wait.
//
// Resty provides the hook and caps the result at RetryMaxWaitTime, but ships no
// implementation: with no callback it always falls back to its own backoff and
// the server's explicit instruction is ignored. Returning 0 hands control back
// to resty's jittered backoff, which is the correct behaviour when the header
// is absent or malformed.
func RetryAfter(_ *resty.Client, resp *resty.Response) (time.Duration, error) {
	if resp == nil {
		return 0, nil
	}

	value := resp.Header().Get("Retry-After")
	if value == "" {
		return 0, nil
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, nil
		}
		return time.Duration(seconds) * time.Second, nil
	}

	if when, err := http.ParseTime(value); err == nil {
		if d := time.Until(when); d > 0 {
			return d, nil
		}
		return 0, nil
	}

	return 0, nil
}
