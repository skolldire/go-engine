package retry_backoff

import (
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

const (
	DefaultInitialWaitTime = 100 * time.Millisecond
	DefaultMaxWaitTime     = 10 * time.Second
	DefaultMaxRetries      = 3
	DefaultBackoffFactor   = 2.0
	DefaultJitterFactor    = 0.2
)

type Retryer struct {
	config *Config
	logger logger.Service
}

type Config struct {
	InitialWaitTime time.Duration `mapstructure:"initial_wait_time" json:"initial_wait_time"`
	MaxWaitTime     time.Duration `mapstructure:"max_wait_time" json:"max_wait_time"`
	MaxRetries      int           `mapstructure:"max_retries" json:"max_retries"`
	BackoffFactor   float64       `mapstructure:"backoff_factor" json:"backoff_factor"`
	JitterFactor    float64       `mapstructure:"jitter_factor" json:"jitter_factor"`
}

type Dependencies struct {
	RetryConfig *Config
	Logger      logger.Service
}
