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

var (
	ErrConnection  = errors.New("database connection error")
	ErrNotFound    = errors.New("record not found")
	ErrTransaction = errors.New("transaction error")
)

// Config holds connection-pool and behaviour settings.
// The caller is responsible for building the gorm.Dialector (and thus the DSN).
type Config struct {
	Type               string            `mapstructure:"type"                json:"type"`
	MaxIdleConnections int               `mapstructure:"max_idle_connections" json:"max_idle_connections"`
	MaxOpenConnections int               `mapstructure:"max_open_connections" json:"max_open_connections"`
	ConnMaxLifetime    time.Duration     `mapstructure:"conn_max_lifetime"    json:"conn_max_lifetime"`
	EnableLogging      bool              `mapstructure:"enable_logging"       json:"enable_logging"`
	LogLevel           string            `mapstructure:"log_level"            json:"log_level"`
	TablePrefix        string            `mapstructure:"table_prefix"         json:"table_prefix"`
	AutoMigrate        bool              `mapstructure:"auto_migrate"         json:"auto_migrate"`
	WithResilience     bool              `mapstructure:"with_resilience"      json:"with_resilience"`
	Resilience         resilience.Config `mapstructure:"resilience"           json:"resilience"`
}

type DBClient struct {
	db         *gorm.DB
	logger     logger.Service
	logging    bool
	resilience *resilience.Service
	dbType     string
}
