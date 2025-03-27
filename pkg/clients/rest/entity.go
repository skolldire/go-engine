package rest

import (
	"context"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"

	"github.com/go-resty/resty/v2"
)

const (
	DefaultRetryCount       = 3
	DefaultRetryWaitTime    = 100 * time.Millisecond
	DefaultRetryMaxWaitTime = 2 * time.Second
	DefaultTimeout          = 10 * time.Second
)

type Config struct {
	BaseURL           string                  `mapstructure:"base_url"`
	TimeOut           time.Duration           `mapstructure:"timeout"`
	EnableLogging     bool                    `mapstructure:"enable_logging"`
	RetryConfig       *retry_backoff.Config   `mapstructure:"retry_config"`
	CircuitBreakerCfg *circuit_breaker.Config `mapstructure:"circuit_breaker_config"`
}

type Service interface {
	Get(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error)
	Post(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error)
	Put(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error)
	Patch(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error)
	Delete(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error)
	WithLogging(enable bool)
}

type client struct {
	baseURL        string
	httpClient     *resty.Client
	retryer        *retry_backoff.Retryer
	circuitBreaker *circuit_breaker.CircuitBreaker
	logger         logger.Service
	logging        bool
}
