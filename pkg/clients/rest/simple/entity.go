package simple

import (
	"context"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

const (
	DefaultRetryCount       = 3
	DefaultRetryWaitTime    = 1 * time.Second
	DefaultRetryMaxWaitTime = 5 * time.Second
	DefaultTimeout          = 5
	DefaultUserAgent        = "uala-bis-go-client/1.0"
)

//go:generate mockery --name Service --output mock --filename service.go
type Service interface {
	Get(ctx context.Context, endpoint string, headers map[string]string, params map[string]string) (*resty.Response, error)
	Post(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error)
	Put(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error)
	Patch(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error)
	Delete(ctx context.Context, endpoint string, headers map[string]string, params map[string]string) (*resty.Response, error)
}

type Config struct {
	BaseURL          string         `mapstructure:"base_url" json:"base_url" validate:"required,url"`
	TimeOut          time.Duration  `mapstructure:"timeout" json:"timeout" validate:"min=1s,max=60s"`
	EnableLogging    bool           `mapstructure:"enable_logging" json:"enable_logging"`
	RetryCount       *int           `mapstructure:"retry_count" json:"retry_count" validate:"min=0,max=10"`
	RetryWaitTime    *time.Duration `mapstructure:"retry_wait_time" json:"retry_wait_time"`
	RetryMaxWaitTime *time.Duration `mapstructure:"retry_max_wait_time" json:"retry_max_wait_time"`
	UserAgent        string         `mapstructure:"user_agent" json:"user_agent"`
}

type Dependencies struct {
	Config Config
	Logger logger.Service
}

func (c *Config) applyDefaults() {
	if c.TimeOut == 0 {
		c.TimeOut = DefaultTimeout * time.Second
	}

	if c.RetryCount == nil {
		def := DefaultRetryCount
		c.RetryCount = &def
	}

	if c.RetryWaitTime == nil {
		def := DefaultRetryWaitTime
		c.RetryWaitTime = &def
	}

	if c.RetryMaxWaitTime == nil {
		def := DefaultRetryMaxWaitTime
		c.RetryMaxWaitTime = &def
	}

	if c.UserAgent == "" {
		c.UserAgent = DefaultUserAgent
	}
}
