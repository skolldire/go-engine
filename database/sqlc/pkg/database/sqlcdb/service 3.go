package sqlcdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	baseclient "github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// New opens a pool with database/sql. The driver must have registered itself,
// which is what the blank import does:
//
//	import _ "github.com/jackc/pgx/v5/stdlib"
//
// The DSN travels in the configuration, so nothing has to be injected in Go.
func New(ctx context.Context, cfg Config, log logger.Service) (*Client, error) {
	if cfg.Driver == "" {
		return nil, log.WrapError(ErrNoDriver, ErrConnection.Error())
	}

	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	return build(ctx, cfg, db, cfg.Driver, log)
}

// NewWithConnector opens a pool from a driver.Connector, for the cases a DSN
// cannot express: credentials rotated at runtime, IAM tokens, a driver
// configured programmatically.
func NewWithConnector(ctx context.Context, cfg Config, connector driver.Connector, log logger.Service) (*Client, error) {
	if connector == nil {
		return nil, log.WrapError(ErrNoDriver, ErrConnection.Error())
	}
	return build(ctx, cfg, sql.OpenDB(connector), cfg.Driver, log)
}

// NewWithDB adopts a pool the caller already owns. The client then drives its
// lifecycle: Close closes this pool.
func NewWithDB(ctx context.Context, cfg Config, db *sql.DB, log logger.Service) (*Client, error) {
	if db == nil {
		return nil, log.WrapError(ErrNoDriver, ErrConnection.Error())
	}
	return build(ctx, cfg, db, cfg.Driver, log)
}

func build(ctx context.Context, cfg Config, db *sql.DB, driverName string, log logger.Service) (*Client, error) {
	maxIdle := DefaultMaxIdleConnections
	if cfg.MaxIdleConnections > 0 {
		maxIdle = cfg.MaxIdleConnections
	}
	db.SetMaxIdleConns(maxIdle)

	maxOpen := DefaultMaxOpenConnections
	if cfg.MaxOpenConnections > 0 {
		maxOpen = cfg.MaxOpenConnections
	}
	db.SetMaxOpenConns(maxOpen)

	lifetime := DefaultConnMaxLifetime
	if cfg.ConnMaxLifetime > 0 {
		lifetime = cfg.ConnMaxLifetime
	}
	db.SetConnMaxLifetime(lifetime)

	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	timeout := DefaultTimeout
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}

	client := &Client{
		db:     db,
		driver: driverName,
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{
			EnableLogging:  cfg.EnableLogging,
			WithResilience: cfg.WithResilience,
			Resilience:     cfg.Resilience,
			Timeout:        timeout,
		}, log, "SQLC"),
	}

	if err := db.PingContext(ctx); err != nil {
		// sql.Open is lazy, so a bad DSN only surfaces here. Close the pool
		// rather than leaking it along with the failed startup.
		_ = db.Close()
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	return client, nil
}

// ExecContext runs a statement through the resilience layer.
//
// With with_resilience enabled the statement may be retried, so enable it only
// for idempotent writes — the retry cannot know an INSERT already landed.
func (c *Client) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	res, err := c.Execute(ctx, "Exec", func(ctx context.Context) (any, error) {
		return c.db.ExecContext(ctx, query, args...)
	})
	if err != nil {
		return nil, err
	}
	return baseclient.SafeTypeAssert[sql.Result](res)
}

// QueryContext runs a query and returns its rows.
//
// It deliberately does not go through BaseClient.Execute. Execute cancels the
// context it derives as soon as it returns, and database/sql keeps *sql.Rows
// bound to the query's context until the rows are closed — the caller would get
// "context canceled" on the first Next(). The caller's own context bounds the
// query instead, and the operation is observed rather than wrapped.
func (c *Client) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := c.db.QueryContext(ctx, query, args...)
	c.trace(ctx, "Query", query, start, err)
	return rows, err
}

// QueryRowContext runs a query returning at most one row. As with QueryContext
// the row stays bound to the caller's context, so the scan is not wrapped; the
// error surfaces on Scan, which is where database/sql puts it.
func (c *Client) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	start := time.Now()
	row := c.db.QueryRowContext(ctx, query, args...)
	c.trace(ctx, "QueryRow", query, start, nil)
	return row
}

// PrepareContext prepares a statement. The context bounds only the preparation,
// so unlike the query methods this one is safe to wrap.
func (c *Client) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	res, err := c.Execute(ctx, "Prepare", func(ctx context.Context) (any, error) {
		return c.db.PrepareContext(ctx, query)
	})
	if err != nil {
		return nil, err
	}
	return baseclient.SafeTypeAssert[*sql.Stmt](res)
}

// Ping checks connectivity. It bypasses the resilience layer on purpose: this
// is what the health check calls, and a retry would report a database that is
// down as merely slow.
func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Tx runs fn inside a transaction, committing when it returns nil and rolling
// back on error or panic.
func (c *Client) Tx(ctx context.Context, fn func(ctx context.Context, tx *sql.Tx) error) error {
	return c.TxWithOptions(ctx, nil, fn)
}

// TxWithOptions is Tx with an explicit isolation level or a read-only
// transaction.
//
// With with_resilience enabled the whole function may run more than once, so fn
// must be safe to replay.
func (c *Client) TxWithOptions(ctx context.Context, opts *sql.TxOptions, fn func(ctx context.Context, tx *sql.Tx) error) error {
	_, err := c.Execute(ctx, "Transaction", func(ctx context.Context) (any, error) {
		tx, err := c.db.BeginTx(ctx, opts)
		if err != nil {
			return nil, err
		}

		defer func() {
			// A panic inside fn must not leave the transaction open holding
			// locks until the connection is reaped.
			if p := recover(); p != nil {
				_ = tx.Rollback()
				panic(p)
			}
		}()

		if err := fn(ctx, tx); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				return nil, errors.Join(err, rbErr)
			}
			return nil, err
		}

		return nil, tx.Commit()
	})
	if err != nil {
		return c.GetLogger().WrapError(err, ErrTransaction.Error())
	}
	return nil
}

// DB returns the underlying pool, for the queries the generated code does not
// cover.
func (c *Client) DB() *sql.DB { return c.db }

// Driver is the driver name the pool was opened with.
func (c *Client) Driver() string { return c.driver }

// Stats reports the pool counters, for metrics or a saturation alert.
func (c *Client) Stats() sql.DBStats { return c.db.Stats() }

// Close closes the pool.
func (c *Client) Close() error { return c.db.Close() }

// InTx runs fn against a sqlc Queries value bound to a transaction. bind is the
// generated WithTx method, which is already the right shape:
//
//	err := sqlcdb.InTx(ctx, client, queries.WithTx,
//	    func(ctx context.Context, q *db.Queries) error {
//	        if err := q.Debit(ctx, from); err != nil {
//	            return err
//	        }
//	        return q.Credit(ctx, to)
//	    })
//
// It exists so the generated code never has to be handed a raw *sql.Tx, which
// is the one place a caller can forget the rollback.
func InTx[Q any](ctx context.Context, c *Client, bind func(*sql.Tx) Q, fn func(ctx context.Context, q Q) error) error {
	return c.Tx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return fn(ctx, bind(tx))
	})
}

// IsNotFound reports whether err is the empty-result error sqlc-generated
// queries return.
func IsNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// trace records an operation that could not be wrapped by the middleware chain.
func (c *Client) trace(ctx context.Context, op, query string, start time.Time, err error) {
	if !c.IsLoggingEnabled() {
		return
	}

	log := c.GetLogger()
	if log == nil {
		return
	}

	fields := map[string]any{
		"operation": op,
		"sql":       query,
		"elapsed":   time.Since(start),
	}
	if err != nil {
		log.Error(ctx, err, fields)
		return
	}
	log.Debug(ctx, "SQL executed", fields)
}
