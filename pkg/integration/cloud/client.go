package cloud

import (
	"context"
)

// Client provides a unified HTTP-like interface for AWS services
// This is the main interface that applications depend on
type Client interface {
	// Do executes an AWS operation with normalized request/response
	// Context is passed here, not in Request
	Do(ctx context.Context, req *Request) (*Response, error)
}

// Middleware is a function that wraps a Client to add cross-cutting concerns
// Examples: logging, metrics, tracing, retries
type Middleware func(next Client) Client
