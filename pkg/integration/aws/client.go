package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/aws/adapters"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/skolldire/go-engine/pkg/integration/observability"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/telemetry"
)

// Client is the AWS implementation of cloud.Client
type Client interface {
	cloud.Client
}

// New creates a new AWS client with conservative defaults
// This is the primary way applications initialize the client
func New(cfg aws.Config) Client {
	return NewWithOptions(cfg, Options{})
}

// Options allows customization (all fields optional)
type Options struct {
	Middlewares []cloud.Middleware // Optional: middleware chain (logging, metrics, tracing)
	Timeout     time.Duration      // Optional: default 30s
	RetryPolicy RetryPolicy        // Optional: retries OFF by default
}

// RetryPolicy controls retry behavior
type RetryPolicy struct {
	Enabled         bool     // Default: false (conservative)
	MaxAttempts     int      // Default: 3
	RetriableErrors []string // Which error codes to retry
}

// NewWithOptions creates a client with custom options
func NewWithOptions(cfg aws.Config, opts Options) Client {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}

	retries := opts.RetryPolicy
	if retries.MaxAttempts == 0 {
		retries.MaxAttempts = 3
	}

	// Create base adapter that handles routing to service adapters
	baseAdapter := adapters.NewBaseAdapter(cfg, timeout, adapters.RetryPolicy{
		Enabled:         retries.Enabled,
		MaxAttempts:     retries.MaxAttempts,
		RetriableErrors: retries.RetriableErrors,
	})

	// Apply middleware chain (observability is optional middleware)
	client := cloud.Client(baseAdapter)
	for _, mw := range opts.Middlewares {
		client = mw(client)
	}

	return client
}

// WithRetry enables retries with sensible defaults
func WithRetry() Options {
	return Options{
		RetryPolicy: RetryPolicy{
			Enabled:     true,
			MaxAttempts: 3,
			RetriableErrors: []string{
				cloud.ErrCodeThrottling,
				cloud.ErrCodeServiceUnavailable,
			},
		},
	}
}

// WithObservability adds logging, metrics, and tracing middleware
func WithObservability(logger logger.Service, metricsRecorder observability.MetricsRecorder, tracer telemetry.Tracer) Options {
	middlewares := []cloud.Middleware{}
	if logger != nil {
		middlewares = append(middlewares, observability.Logging(logger))
	}
	if metricsRecorder != nil {
		middlewares = append(middlewares, observability.Metrics(metricsRecorder))
	}
	if tracer != nil {
		middlewares = append(middlewares, observability.Tracing(tracer))
	}
	return Options{Middlewares: middlewares}
}

