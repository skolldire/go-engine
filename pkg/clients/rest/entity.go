package rest

import (
	"context"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultRetryCount       = 3
	DefaultRetryWaitTime    = 100 * time.Millisecond
	DefaultRetryMaxWaitTime = 2 * time.Second
	DefaultTimeout          = 10 * time.Second
)

type Config struct {
	BaseURL        string            `mapstructure:"base_url" json:"base_url"`
	TimeOut        time.Duration     `mapstructure:"timeout" json:"time_out"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
}

type Service interface {
	Get(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error)
	Post(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error)
	Put(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error)
	Patch(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (*resty.Response, error)
	Delete(ctx context.Context, endpoint string, headers map[string]string) (*resty.Response, error)
	WithLogging(enable bool)
}

type restClient struct {
	*client.BaseClient
	baseURL    string
	httpClient *resty.Client
}
