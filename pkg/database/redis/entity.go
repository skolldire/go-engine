package redis

import (
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
)

const (
	DefaultTimeout    = 30 * time.Second
	DefaultExpiration = 24 * time.Hour
)

var (
	ErrKeyNotFound  = errors.New("clave no encontrada")
	ErrInvalidValue = errors.New("valor inválido")
	ErrConnection   = errors.New("error de conexión con redis")
)

type Config struct {
	Host              string                  `mapstructure:"host"`
	Port              int                     `mapstructure:"port"`
	DB                int                     `mapstructure:"db"`
	Password          string                  `mapstructure:"password"`
	Timeout           int                     `mapstructure:"timeout"`
	Prefix            string                  `mapstructure:"prefix"`
	EnableLogging     bool                    `mapstructure:"enable_logging"`
	RetryConfig       *retry_backoff.Config   `mapstructure:"retry_config"`
	CircuitBreakerCfg *circuit_breaker.Config `mapstructure:"circuit_breaker_config"`
}

type RedisClient struct {
	client         *redis.Client
	logger         logger.Service
	logging        bool
	retryer        *retry_backoff.Retryer
	circuitBreaker *circuit_breaker.CircuitBreaker
	keyPrefix      string
}
