package rest

import (
	"context"
	"errors"
	"net/http"

	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// Circuit breaking is the one piece of resilience resty v2 does not provide.
// Rather than write another breaker, this wires the existing gobreaker wrapper
// in pkg/utilities/circuit_breaker into resty's transport.
//
// The placement is the whole point. Resty's retry loop calls the underlying
// http.Client once per attempt, so a RoundTripper sits *inside* the retry loop:
// the breaker observes individual attempts, opens on the real failure count,
// and once open it rejects immediately, which ends the retry loop instead of
// letting it keep hammering a service that is already down.
//
// Wrapping the resty call from the outside would give the opposite composition:
// three attempts counted as a single failure, and three times as many requests
// needed before the upstream is protected.

// ErrCircuitOpen is returned while the breaker is open.
var ErrCircuitOpen = circuit_breaker.ErrCircuitOpen

// breakerTransport is an http.RoundTripper that admits or rejects each attempt.
type breakerTransport struct {
	next http.RoundTripper
	cb   *circuit_breaker.CircuitBreaker
}

// newBreakerTransport wraps next with the breaker described by cfg.
func newBreakerTransport(next http.RoundTripper, cfg *circuit_breaker.Config, log logger.Service) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &breakerTransport{
		next: next,
		cb: circuit_breaker.NewCircuitBreaker(circuit_breaker.Dependencies{
			Config: cfg,
			Log:    log,
		}),
	}
}

// RoundTrip implements http.RoundTripper.
func (t *breakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// The response is captured here rather than taken from Execute's return
	// value: circuit_breaker.Execute discards the result whenever the operation
	// reports an error, and this transport reports one on purpose for statuses
	// that should count against the breaker.
	var resp *http.Response

	_, err := t.cb.Execute(ctx, func() (any, error) {
		r, rtErr := t.next.RoundTrip(req)
		resp = r

		if rtErr != nil {
			return nil, rtErr
		}

		// Decide what counts against the breaker. A 404 means the dependency is
		// up and answering; counting it would open the circuit because of our
		// own bad request rather than the upstream's health. Only the statuses
		// that indicate a transient server-side condition are failures.
		if r != nil && RetryableStatus(r.StatusCode) {
			return nil, errUnhealthyStatus
		}
		return nil, nil
	})

	switch {
	case errors.Is(err, errUnhealthyStatus):
		// Informational only: the response is valid and the caller decides what
		// a 503 means. Returning it with a nil error keeps the RoundTripper
		// contract, which forbids a nil response with a nil error.
		return resp, nil
	case err != nil:
		// Circuit open, or a genuine transport failure.
		return nil, err
	default:
		return resp, nil
	}
}

// errUnhealthyStatus marks a response as a failure for the breaker without
// becoming the caller's error.
var errUnhealthyStatus = errors.New("upstream reported an unhealthy status")
