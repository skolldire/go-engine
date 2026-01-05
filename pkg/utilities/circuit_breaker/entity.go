package circuit_breaker

import (
	"errors"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/sony/gobreaker"
)

const (
	DefaultCBName                 = "default-circuit-breaker"
	DefaultCBMaxRequests          = 100
	DefaultCBInterval             = 60 * time.Second
	DefaultCBTimeout              = 30 * time.Second
	DefaultCBRequestThreshold     = 5
	DefaultCBFailureRateThreshold = 0.5
)

var (
	ErrCircuitOpen  = errors.New("circuit breaker is open")
	ErrTooManyCalls = errors.New("too many concurrent calls")
)

type Config struct {
	Name                 string        `mapstructure:"name" json:"name"`
	MaxRequests          uint32        `mapstructure:"max_requests" json:"max_requests"`
	Interval             time.Duration `mapstructure:"interval" json:"interval"`
	Timeout              time.Duration `mapstructure:"timeout" json:"timeout"`
	RequestThreshold     uint32        `mapstructure:"request_threshold" json:"request_threshold"`
	FailureRateThreshold float64       `mapstructure:"failure_rate_threshold" json:"failure_rate_threshold"`
}

type CircuitBreaker struct {
	cb     *gobreaker.CircuitBreaker
	config *Config
	log    logger.Service
}

type Dependencies struct {
	Config *Config
	Log    logger.Service
}
