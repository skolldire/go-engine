package gormsql

import (
	"context"
	"errors"
	"fmt"
	"time"

	baseclient "github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// New opens a GORM connection using the caller-supplied dialector.
// The caller is responsible for importing the appropriate driver and building
// the dialector (e.g. postgres.Open(dsn), mysql.Open(dsn)).
func New(ctx context.Context, cfg Config, dialector gorm.Dialector, log logger.Service) (*DBClient, error) {
	gormConfig := &gorm.Config{}

	if cfg.TablePrefix != "" {
		gormConfig.NamingStrategy = schema.NamingStrategy{
			TablePrefix: cfg.TablePrefix,
		}
	}

	if cfg.EnableLogging {
		gormConfig.Logger = createGormLogger(log, cfg.LogLevel)
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	maxIdle := DefaultMaxIdleConnections
	if cfg.MaxIdleConnections > 0 {
		maxIdle = cfg.MaxIdleConnections
	}
	sqlDB.SetMaxIdleConns(maxIdle)

	maxOpen := DefaultMaxOpenConnections
	if cfg.MaxOpenConnections > 0 {
		maxOpen = cfg.MaxOpenConnections
	}
	sqlDB.SetMaxOpenConns(maxOpen)

	lifetime := DefaultConnMaxLifetime
	if cfg.ConnMaxLifetime > 0 {
		lifetime = cfg.ConnMaxLifetime
	}
	sqlDB.SetConnMaxLifetime(lifetime)

	client := &DBClient{
		db:     db,
		dbType: cfg.Type,
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{
			EnableLogging:  cfg.EnableLogging,
			WithResilience: cfg.WithResilience,
			Resilience:     cfg.Resilience,
			Timeout:        DefaultTimeout,
		}, log, "SQL"),
	}

	if err := sqlDB.Ping(); err != nil {
		// Close the underlying *sql.DB so its connection pool is not leaked
		// when the initial handshake fails.
		_ = sqlDB.Close()
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	return client, nil
}

func (dbc *DBClient) Ping(ctx context.Context) error {
	sqlDB, err := dbc.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (dbc *DBClient) WithContext(ctx context.Context) *gorm.DB {
	return dbc.db.WithContext(ctx)
}

func (dbc *DBClient) Create(ctx context.Context, value any) error {
	_, err := dbc.Execute(ctx, "Create", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Create(value).Error
	})
	return err
}

func (dbc *DBClient) First(ctx context.Context, dest any, conditions ...any) error {
	_, err := dbc.Execute(ctx, "First", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).First(dest, conditions...).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Wrap rather than replace: callers matching this package's ErrNotFound
		// keep working, and a caller that already imports gorm can still match
		// gorm.ErrRecordNotFound. Returning a bare sentinel with an identical
		// message broke the chain for the second group with no way to tell.
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}

func (dbc *DBClient) Find(ctx context.Context, dest any, conditions ...any) error {
	_, err := dbc.Execute(ctx, "Find", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Find(dest, conditions...).Error
	})
	return err
}

func (dbc *DBClient) Update(ctx context.Context, model any, updates any) error {
	_, err := dbc.Execute(ctx, "Update", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Model(model).Updates(updates).Error
	})
	return err
}

func (dbc *DBClient) Delete(ctx context.Context, value any, conditions ...any) error {
	_, err := dbc.Execute(ctx, "Delete", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Delete(value, conditions...).Error
	})
	return err
}

func (dbc *DBClient) Count(ctx context.Context, model any, count *int64, conditions ...any) error {
	_, err := dbc.Execute(ctx, "Count", func(ctx context.Context) (any, error) {
		q := dbc.db.WithContext(ctx).Model(model)
		if len(conditions) > 0 {
			q = q.Where(conditions[0], conditions[1:]...)
		}
		return nil, q.Count(count).Error
	})
	return err
}

func (dbc *DBClient) Exec(ctx context.Context, sql string, values ...any) error {
	_, err := dbc.Execute(ctx, "Exec", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Exec(sql, values...).Error
	})
	return err
}

func (dbc *DBClient) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	_, err := dbc.Execute(ctx, "Transaction", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Transaction(fn)
	})
	if err != nil {
		return dbc.GetLogger().WrapError(err, ErrTransaction.Error())
	}
	return nil
}

func (dbc *DBClient) Preload(ctx context.Context, dest any, relation string, conditions ...any) error {
	_, err := dbc.Execute(ctx, "Preload", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Preload(relation, conditions...).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Where(ctx context.Context, dest any, query any, args ...any) error {
	_, err := dbc.Execute(ctx, "Where", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Where(query, args...).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Order(ctx context.Context, dest any, value any) error {
	_, err := dbc.Execute(ctx, "Order", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Order(value).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Limit(ctx context.Context, dest any, limit int) error {
	_, err := dbc.Execute(ctx, "Limit", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Limit(limit).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Offset(ctx context.Context, dest any, offset int) error {
	_, err := dbc.Execute(ctx, "Offset", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Offset(offset).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Upsert(ctx context.Context, value any, conflictColumns []string, updateColumns []string) error {
	cols := make([]clause.Column, len(conflictColumns))
	for i, c := range conflictColumns {
		cols[i] = clause.Column{Name: c}
	}
	_, err := dbc.Execute(ctx, "Upsert", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   cols,
			DoUpdates: clause.AssignmentColumns(updateColumns),
		}).Create(value).Error
	})
	return err
}

func (dbc *DBClient) AutoMigrate(models ...any) error {
	if err := dbc.db.AutoMigrate(models...); err != nil {
		return dbc.GetLogger().WrapError(err, "error in auto migration")
	}
	return nil
}

func (dbc *DBClient) Raw(ctx context.Context, dest any, sql string, values ...any) error {
	_, err := dbc.Execute(ctx, "Raw", func(ctx context.Context) (any, error) {
		return nil, dbc.db.WithContext(ctx).Raw(sql, values...).Scan(dest).Error
	})
	return err
}

func (dbc *DBClient) DB() *gorm.DB {
	return dbc.db
}

func (dbc *DBClient) Close() error {
	sqlDB, err := dbc.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// ── gorm logger adapter ───────────────────────────────────────────────────────

type gormLogAdapter struct {
	logger   logger.Service
	logLevel string
}

func (l *gormLogAdapter) LogMode(_ gormlogger.LogLevel) gormlogger.Interface { return l }

func (l *gormLogAdapter) Info(ctx context.Context, msg string, data ...any) {
	l.logger.Info(ctx, fmt.Sprintf(msg, data...), map[string]any{"type": "info"})
}

func (l *gormLogAdapter) Warn(ctx context.Context, msg string, data ...any) {
	l.logger.Warn(ctx, fmt.Sprintf(msg, data...), map[string]any{"type": "warn"})
}

func (l *gormLogAdapter) Error(ctx context.Context, msg string, data ...any) {
	l.logger.Error(ctx, fmt.Errorf(msg, data...), map[string]any{"type": "error"})
}

func (l *gormLogAdapter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	fields := map[string]any{
		"elapsed": time.Since(begin),
		"rows":    rows,
		"sql":     sql,
	}
	if err != nil {
		l.logger.Error(ctx, err, fields)
		return
	}
	l.logger.Debug(ctx, "SQL executed", fields)
}

func createGormLogger(log logger.Service, logLevel string) gormlogger.Interface {
	return &gormLogAdapter{logger: log, logLevel: logLevel}
}
