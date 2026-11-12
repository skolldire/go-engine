// Package rest provides an HTTP client with retry, circuit breaker, and logging support.
package rest

import (
	"context"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
)

const (
	DefaultRetryCount       = 3
	DefaultRetryWaitTime    = 100 * time.Millisecond
	DefaultRetryMaxWaitTime = 2 * time.Second
	DefaultTimeout          = 10 * time.Second
)

type Config struct {
	BaseURL       string        `mapstructure:"base_url" json:"base_url"`
	TimeOut       time.Duration `mapstructure:"timeout" json:"time_out"`
	EnableLogging bool          `mapstructure:"enable_logging" json:"enable_logging"`

	// Retry configures resty's retry loop. A nil block means no retries at all,
	// so resilience is something you compose in rather than a feature that is
	// always in the call path behind a boolean.
	Retry *RetryConfig `mapstructure:"retry" json:"retry"`

	// CircuitBreaker adds the breaker resty does not provide. Nil means none.
	CircuitBreaker *circuit_breaker.Config `mapstructure:"circuit_breaker" json:"circuit_breaker"`
}

// RetryConfig configures resty's own retry engine. The exponential backoff, its
// jitter and the cap on Retry-After are resty's; only the policy of what may be
// replayed is ours.
type RetryConfig struct {
	// MaxRetries is the number of retries after the first attempt.
	MaxRetries int `mapstructure:"max_retries" json:"max_retries"`
	// WaitTime is the initial backoff interval.
	WaitTime time.Duration `mapstructure:"wait_time" json:"wait_time"`
	// MaxWaitTime caps a single backoff interval, and also caps how long a
	// Retry-After header may hold the request.
	MaxWaitTime time.Duration `mapstructure:"max_wait_time" json:"max_wait_time"`
	// RetryNonIdempotent also replays POST and PATCH. Off by default: replaying
	// one can duplicate a side effect the server already applied.
	RetryNonIdempotent bool `mapstructure:"retry_non_idempotent" json:"retry_non_idempotent"`
}

type Service interface {
	// R returns a resty request builder bound to this client, already carrying
	// the base URL, timeout, retry policy and circuit breaker.
	//
	// This is the full builder — path parameters, query parameters, headers,
	// multipart, file uploads, response unmarshalling — rather than a
	// reimplementation of it. The previous fixed method set hid all of it behind
	// (endpoint, headers), so callers could not set a query parameter without
	// concatenating it into the path by hand.
	//
	//	resp, err := c.R(ctx).
	//	    SetPathParam("id", "42").
	//	    SetQueryParam("page", "2").
	//	    SetHeader("X-Trace", traceID).
	//	    SetBody(payload).
	//	    Post("/users/{id}/orders")
	R(ctx context.Context) *resty.Request

	// The verb helpers remain for the common case. Unlike before, they return
	// the response alongside the error on a non-2xx, so a caller can read the
	// status and body without parsing an error message.
	Get(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error)
	Post(ctx context.Context, endpoint string, body any, headers map[string]string) (*resty.Response, error)
	Put(ctx context.Context, endpoint string, body any, headers map[string]string) (*resty.Response, error)
	Patch(ctx context.Context, endpoint string, body any, headers map[string]string) (*resty.Response, error)
	Delete(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error)
	WithLogging(enable bool)
}

type restClient struct {
	*client.BaseClient
	baseURL    string
	httpClient *resty.Client
}
