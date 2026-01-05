package gormsql

import (
	"errors"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
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
	ErrInvalidDBType = errors.New("unsupported database type")
	ErrConnection    = errors.New("database connection error")
	ErrNotFound      = errors.New("record not found")
	ErrTransaction   = errors.New("transaction error")
)

type Config struct {
	Type               string            `mapstructure:"type" json:"type"`
	Host               string            `mapstructure:"host" json:"host"`
	Port               int               `mapstructure:"port" json:"port"`
	Username           string            `mapstructure:"username" json:"username"`
	Password           string            `mapstructure:"password" json:"password"`
	Database           string            `mapstructure:"database" json:"database"`
	SSLMode            string            `mapstructure:"ssl_mode" json:"ssl_modeMode"`
	MaxIdleConnections int               `mapstructure:"max_idle_connections" json:"max_idle_connections"`
	MaxOpenConnections int               `mapstructure:"max_open_connections" json:"max_open_connections"`
	ConnMaxLifetime    time.Duration     `mapstructure:"conn_max_lifetime" json:"conn_max_lifetime"`
	EnableLogging      bool              `mapstructure:"enable_logging" json:"enable_logging"`
	LogLevel           string            `mapstructure:"log_level" json:"log_level"`
	TablePrefix        string            `mapstructure:"table_prefix" json:"table_prefix"`
	AutoMigrate        bool              `mapstructure:"auto_migrate" json:"auto_migrate"`
	WithResilience     bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience         resilience.Config `mapstructure:"resilience" json:"resilience"`
}

type DBClient struct {
	db         *gorm.DB
	logger     logger.Service
	logging    bool
	resilience *resilience.Service
	dbType     string
}
