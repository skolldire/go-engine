package gormsql

import (
	"errors"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
	"gorm.io/gorm"
)

const (
	DefaultConnMaxLifetime    = 5 * time.Minute
	DefaultMaxIdleConnections = 10
	DefaultMaxOpenConnections = 100
	DefaultTimeout            = 30 * time.Second
)

const (
	PostgresSQL = "postgres"
	MySQL       = "mysql"
	SQLite      = "sqlite"
	SQLServer   = "sqlserver"
)

var (
	ErrInvalidDBType = errors.New("tipo de base de datos no soportado")
	ErrConnection    = errors.New("error de conexión con la base de datos")
	ErrNotFound      = errors.New("registro no encontrado")
	ErrTransaction   = errors.New("error en la transacción")
)

type Config struct {
	Type               string                  `mapstructure:"type"`
	Host               string                  `mapstructure:"host"`
	Port               int                     `mapstructure:"port"`
	Username           string                  `mapstructure:"username"`
	Password           string                  `mapstructure:"password"`
	Database           string                  `mapstructure:"database"`
	SSLMode            string                  `mapstructure:"sslmode"`
	MaxIdleConnections int                     `mapstructure:"max_idle_connections"`
	MaxOpenConnections int                     `mapstructure:"max_open_connections"`
	ConnMaxLifetime    time.Duration           `mapstructure:"conn_max_lifetime"`
	EnableLogging      bool                    `mapstructure:"enable_logging"`
	LogLevel           string                  `mapstructure:"log_level"`
	TablePrefix        string                  `mapstructure:"table_prefix"`
	AutoMigrate        bool                    `mapstructure:"auto_migrate"`
	RetryConfig        *retry_backoff.Config   `mapstructure:"retry_config"`
	CircuitBreakerCfg  *circuit_breaker.Config `mapstructure:"circuit_breaker_config"`
}

type DBClient struct {
	db             *gorm.DB
	logger         logger.Service
	logging        bool
	retryer        *retry_backoff.Retryer
	circuitBreaker *circuit_breaker.CircuitBreaker
	dbType         string
}
