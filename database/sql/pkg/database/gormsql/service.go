package gormsql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// New opens a GORM connection using the caller-supplied dialector.
// The caller is responsible for importing the appropriate driver and building
// the dialector (e.g. postgres.Open(dsn), mysql.Open(dsn)).
func New(cfg Config, dialector gorm.Dialector, log logger.Service) (*DBClient, error) {
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
		db:      db,
		logger:  log,
		logging: cfg.EnableLogging,
		dbType:  cfg.Type,
	}

	if cfg.WithResilience {
		client.resilience = resilience.NewResilienceService(cfg.Resilience, log)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	if client.logging {
		log.Debug(context.Background(), fmt.Sprintf("database connection to %s established", cfg.Type),
			map[string]interface{}{"type": cfg.Type})
	}

	return client, nil
}

func (dbc *DBClient) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); !ok {
		return context.WithTimeout(ctx, DefaultTimeout)
	}
	return context.WithCancel(ctx)
}

func (dbc *DBClient) execute(ctx context.Context, op string, fn func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := dbc.ensureContextWithTimeout(ctx)
	defer cancel()

	fields := map[string]interface{}{"operation": op, "db_type": dbc.dbType}

	if dbc.resilience != nil {
		if dbc.logging {
			dbc.logger.Debug(ctx, fmt.Sprintf("starting DB operation with resilience: %s", op), fields)
		}
		result, err := dbc.resilience.Execute(ctx, fn)
		if err != nil && dbc.logging {
			dbc.logger.Error(ctx, fmt.Errorf("error in DB operation: %w", err), fields)
		} else if dbc.logging {
			dbc.logger.Debug(ctx, fmt.Sprintf("DB operation completed with resilience: %s", op), fields)
		}
		return result, err
	}

	if dbc.logging {
		dbc.logger.Debug(ctx, fmt.Sprintf("starting DB operation: %s", op), fields)
	}
	result, err := fn()
	if err != nil && dbc.logging {
		dbc.logger.Error(ctx, err, fields)
	} else if dbc.logging {
		dbc.logger.Debug(ctx, fmt.Sprintf("DB operation completed: %s", op), fields)
	}
	return result, err
}

// Ping verifies database connectivity using the underlying sql.DB.
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

func (dbc *DBClient) Create(ctx context.Context, value interface{}) error {
	_, err := dbc.execute(ctx, "Create", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Create(value).Error
	})
	return err
}

func (dbc *DBClient) First(ctx context.Context, dest interface{}, conditions ...interface{}) error {
	_, err := dbc.execute(ctx, "First", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).First(dest, conditions...).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func (dbc *DBClient) Find(ctx context.Context, dest interface{}, conditions ...interface{}) error {
	_, err := dbc.execute(ctx, "Find", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Find(dest, conditions...).Error
	})
	return err
}

func (dbc *DBClient) Update(ctx context.Context, model interface{}, updates interface{}) error {
	_, err := dbc.execute(ctx, "Update", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Model(model).Updates(updates).Error
	})
	return err
}

func (dbc *DBClient) Delete(ctx context.Context, value interface{}, conditions ...interface{}) error {
	_, err := dbc.execute(ctx, "Delete", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Delete(value, conditions...).Error
	})
	return err
}

func (dbc *DBClient) Count(ctx context.Context, model interface{}, count *int64, conditions ...interface{}) error {
	_, err := dbc.execute(ctx, "Count", func() (interface{}, error) {
		q := dbc.db.WithContext(ctx).Model(model)
		if len(conditions) > 0 {
			q = q.Where(conditions[0], conditions[1:]...)
		}
		return nil, q.Count(count).Error
	})
	return err
}

func (dbc *DBClient) Exec(ctx context.Context, sql string, values ...interface{}) error {
	_, err := dbc.execute(ctx, "Exec", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Exec(sql, values...).Error
	})
	return err
}

func (dbc *DBClient) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	_, err := dbc.execute(ctx, "Transaction", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Transaction(fn)
	})
	if err != nil {
		return dbc.logger.WrapError(err, ErrTransaction.Error())
	}
	return nil
}

func (dbc *DBClient) Preload(ctx context.Context, dest interface{}, relation string, conditions ...interface{}) error {
	_, err := dbc.execute(ctx, "Preload", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Preload(relation, conditions...).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Where(ctx context.Context, dest interface{}, query interface{}, args ...interface{}) error {
	_, err := dbc.execute(ctx, "Where", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Where(query, args...).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Order(ctx context.Context, dest interface{}, value interface{}) error {
	_, err := dbc.execute(ctx, "Order", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Order(value).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Limit(ctx context.Context, dest interface{}, limit int) error {
	_, err := dbc.execute(ctx, "Limit", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Limit(limit).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Offset(ctx context.Context, dest interface{}, offset int) error {
	_, err := dbc.execute(ctx, "Offset", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Offset(offset).Find(dest).Error
	})
	return err
}

func (dbc *DBClient) Upsert(ctx context.Context, value interface{}, conflictColumns []string, updateColumns []string) error {
	cols := make([]clause.Column, len(conflictColumns))
	for i, c := range conflictColumns {
		cols[i] = clause.Column{Name: c}
	}
	_, err := dbc.execute(ctx, "Upsert", func() (interface{}, error) {
		return nil, dbc.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   cols,
			DoUpdates: clause.AssignmentColumns(updateColumns),
		}).Create(value).Error
	})
	return err
}

func (dbc *DBClient) AutoMigrate(models ...interface{}) error {
	if err := dbc.db.AutoMigrate(models...); err != nil {
		return dbc.logger.WrapError(err, "error in auto migration")
	}
	return nil
}

func (dbc *DBClient) Raw(ctx context.Context, dest interface{}, sql string, values ...interface{}) error {
	_, err := dbc.execute(ctx, "Raw", func() (interface{}, error) {
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

func (l *gormLogAdapter) Info(ctx context.Context, msg string, data ...interface{}) {
	l.logger.Info(ctx, fmt.Sprintf(msg, data...), map[string]interface{}{"type": "info"})
}

func (l *gormLogAdapter) Warn(ctx context.Context, msg string, data ...interface{}) {
	l.logger.Warn(ctx, fmt.Sprintf(msg, data...), map[string]interface{}{"type": "warn"})
}

func (l *gormLogAdapter) Error(ctx context.Context, msg string, data ...interface{}) {
	l.logger.Error(ctx, errors.New(fmt.Sprintf(msg, data...)), map[string]interface{}{"type": "error"})
}

func (l *gormLogAdapter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	fields := map[string]interface{}{
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
