package otel

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// NewMiddleware returns a chi-compatible HTTP middleware that instruments
// requests with distributed tracing and latency metrics.
// When cfg.Enabled is false, requests pass through without instrumentation.
func NewMiddleware(cfg OTELConfig) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}
	return otelhttp.NewMiddleware(cfg.ServiceName)
}
