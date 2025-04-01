package redis

import (
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
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
	Host           string            `mapstructure:"host" json:"host"`
	Port           int               `mapstructure:"port" json:"port"`
	DB             int               `mapstructure:"db" json:"db"`
	Password       string            `mapstructure:"password" json:"password"`
	Timeout        int               `mapstructure:"timeout" json:"timeout"`
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
