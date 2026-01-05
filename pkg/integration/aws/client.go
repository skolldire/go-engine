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

// awsClient implements cloud.Client
type awsClient struct {
	config   aws.Config
	timeout  time.Duration
	retries  RetryPolicy
	base     cloud.Client // Base client (before middleware)
}

// New creates a new AWS client with conservative defaults
// New creates an AWS client configured with cfg and default options.
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

// NewWithOptions creates a Client configured with the provided AWS config and Options.
// It uses opts.Timeout (defaults to 30s if zero) and opts.RetryPolicy (defaults MaxAttempts to 3 if zero)
// to construct the underlying adapter's retry and timeout settings, then applies opts.Middlewares in order
// to produce the final cloud.Client.
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

// Do implements cloud.Client interface
func (c *awsClient) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// This should not be called directly - middleware chain handles it
	// But we need to implement the interface
	return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "awsClient.Do should not be called directly")
}

// WithRetry returns Options that enable retry behavior using sensible defaults.
// The returned Options enables retries, sets MaxAttempts to 3, and treats
// throttling and service-unavailable error codes as retriable.
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

// WithObservability returns Options containing observability middlewares.
// WithObservability adds logging, metrics, and tracing middleware for the
// provided collaborators; any nil collaborator is ignored.
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
