package sqlcdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	baseclient "github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultConnMaxLifetime    = 5 * time.Minute
	DefaultMaxIdleConnections = 10
	DefaultMaxOpenConnections = 100
	DefaultTimeout            = 30 * time.Second
)

var (
	ErrConnection  = errors.New("database connection error")
	ErrTransaction = errors.New("transaction error")
	ErrNoDriver    = errors.New("no driver configured")

	// ErrNotFound is sql.ErrNoRows, not a sentinel of this package's own.
	// sqlc-generated queries return sql.ErrNoRows directly, so a distinct
	// sentinel would only be matchable by code that never sees it: a caller
	// writing errors.Is(err, sqlcdb.ErrNotFound) around q.GetUser(...) has to
	// be matching the same error the generated code returns.
	ErrNotFound = sql.ErrNoRows
)

// Config holds the driver, the DSN and the connection-pool settings.
//
// Unlike the GORM client, the driver *can* come from configuration here: a
// database/sql driver is named by a string and registers itself through a blank
// import, so nothing has to be linked that the application did not import.
type Config struct {
	Driver             string            `mapstructure:"driver"               json:"driver"`
	DSN                string            `mapstructure:"dsn"                  json:"dsn"`
	MaxIdleConnections int               `mapstructure:"max_idle_connections" json:"max_idle_connections"`
	MaxOpenConnections int               `mapstructure:"max_open_connections" json:"max_open_connections"`
	ConnMaxLifetime    time.Duration     `mapstructure:"conn_max_lifetime"    json:"conn_max_lifetime"`
	ConnMaxIdleTime    time.Duration     `mapstructure:"conn_max_idle_time"   json:"conn_max_idle_time"`
	EnableLogging      bool              `mapstructure:"enable_logging"       json:"enable_logging"`
	Timeout            time.Duration     `mapstructure:"timeout"              json:"timeout"`
	WithResilience     bool              `mapstructure:"with_resilience"      json:"with_resilience"`
	Resilience         resilience.Config `mapstructure:"resilience"           json:"resilience"`
}

// DBTX is the handle every sqlc-generated package requires. It is declared here
// with exactly the shape sqlc emits, so *Client can be handed straight to the
// generated New(db DBTX) without an adapter in between:
//
//	queries := db.New(client)   // client is a *sqlcdb.Client
//
// Both *Client and *sql.Tx satisfy it, which is what lets the same generated
// query run inside and outside a transaction.
type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Service is the contract of the client. It is DBTX plus the lifecycle the
// engine drives, so a consumer can depend on the interface and inject a double.
type Service interface {
	DBTX

	Ping(ctx context.Context) error
	Tx(ctx context.Context, fn func(ctx context.Context, tx *sql.Tx) error) error
	TxWithOptions(ctx context.Context, opts *sql.TxOptions, fn func(ctx context.Context, tx *sql.Tx) error) error
	DB() *sql.DB
	Driver() string
	Stats() sql.DBStats
	Close() error
}

// Client is a *sql.DB with the framework's logging, timeout and resilience
// wrapped around the statement-executing methods.
type Client struct {
	db     *sql.DB
	driver string
	*baseclient.BaseClient
}

var (
	_ Service = (*Client)(nil)
	_ DBTX    = (*sql.Tx)(nil)
)
