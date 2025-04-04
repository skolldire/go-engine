package redis

import (
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout      = 30 * time.Second
	DefaultDialTimeout  = 5 * time.Second
	DefaultReadTimeout  = 3 * time.Second
	DefaultWriteTimeout = 3 * time.Second
	DefaultPoolSize     = 10
	DefaultExpiration   = 24 * time.Hour
)

var (
	ErrKeyNotFound  = errors.New("clave no encontrada")
	ErrInvalidValue = errors.New("valor inválido")
	ErrConnection   = errors.New("error de conexión con redis")
)

type Config struct {
	Host           string            `mapstructure:"host" json:"host"`
	Port           int               `mapstructure:"port" json:"port"`
	DB             int               `mapstructure:"db" json:"db"`
	Password       string            `mapstructure:"password" json:"password"`
	Timeout        time.Duration     `mapstructure:"timeout" json:"timeout"`
	DialTimeout    time.Duration     `mapstructure:"dial_timeout" json:"dial_timeout"`
	ReadTimeout    time.Duration     `mapstructure:"read_timeout" json:"read_timeout"`
	WriteTimeout   time.Duration     `mapstructure:"write_timeout" json:"write_timeout"`
	PoolSize       int               `mapstructure:"pool_size" json:"pool_size"`
	Prefix         string            `mapstructure:"prefix" json:"prefix"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
}

type RedisClient struct {
	client     *redis.Client
	logger     logger.Service
	logging    bool
	resilience *resilience.Service
	keyPrefix  string
}
