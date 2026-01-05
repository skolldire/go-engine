package gormsql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// NewClient creates and configures a GORM-based DBClient using the provided configuration and logger.
// 
// It initializes optional naming (TablePrefix) and GORM logging, opens a connection for the configured
// database type (PostgresSQL, MySQL, SQLite, or SQLServer), configures connection pool parameters
// (max idle/open connections and connection max lifetime), and optionally attaches a resilience service.
// The function verifies the database connection with a ping before returning the client.
//
// Errors:
// - returns ErrInvalidDBType if cfg.Type is not one of the supported database types.
// - returns a wrapped ErrConnection when opening the database, obtaining the underlying sql.DB, or pinging the database fails.
func NewClient(cfg Config, log logger.Service) (*DBClient, error) {
	var db *gorm.DB
	var err error

	gormConfig := &gorm.Config{}
	if cfg.TablePrefix != "" {
		gormConfig.NamingStrategy = schema.NamingStrategy{
			TablePrefix: cfg.TablePrefix,
		}
	}

	if cfg.EnableLogging {
		gormLogger := createGormLogger(log, cfg.LogLevel)
		gormConfig.Logger = gormLogger
	}

	switch cfg.Type {
	case PostgresSQL:
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database, cfg.SSLMode)
		db, err = gorm.Open(postgres.Open(dsn), gormConfig)

	case MySQL:
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
		db, err = gorm.Open(mysql.Open(dsn), gormConfig)

	case SQLite:
		db, err = gorm.Open(sqlite.Open(cfg.Database), gormConfig)

	case SQLServer:
		dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
		db, err = gorm.Open(sqlserver.Open(dsn), gormConfig)

	default:
		return nil, ErrInvalidDBType
	}

	if err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	maxIdleConnections := DefaultMaxIdleConnections
	if cfg.MaxIdleConnections > 0 {
		maxIdleConnections = cfg.MaxIdleConnections
	}
	sqlDB.SetMaxIdleConns(maxIdleConnections)

	maxOpenConnections := DefaultMaxOpenConnections
	if cfg.MaxOpenConnections > 0 {
		maxOpenConnections = cfg.MaxOpenConnections
	}
	sqlDB.SetMaxOpenConns(maxOpenConnections)

	connMaxLifetime := DefaultConnMaxLifetime
	if cfg.ConnMaxLifetime > 0 {
		connMaxLifetime = cfg.ConnMaxLifetime
	}
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	client := &DBClient{
		db:      db,
		logger:  log,
		logging: cfg.EnableLogging,
		dbType:  cfg.Type,
	}

	if cfg.WithResilience {
		resilienceService := resilience.NewResilienceService(cfg.Resilience, log)
		client.resilience = resilienceService
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	if client.logging {
		msg := fmt.Sprintf("database connection to %s established", cfg.Type)
		logFields := map[string]interface{}{"type": cfg.Type, "database": cfg.Database}
		log.Debug(context.Background(), msg, logFields)
	}

	return client, nil
}

func (dbc *DBClient) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return context.WithTimeout(ctx, DefaultTimeout)
	}
	return context.WithCancel(ctx)
}

func (dbc *DBClient) execute(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := dbc.ensureContextWithTimeout(ctx)
	defer cancel()

	logFields := map[string]interface{}{"operation": operationName, "db_type": dbc.dbType}

	if dbc.resilience != nil {
		if dbc.logging {
			dbc.logger.Debug(ctx, fmt.Sprintf("starting DB operation with resilience: %s", operationName), logFields)
		}

		result, err := dbc.resilience.Execute(ctx, operation)

		if err != nil && dbc.logging {
			dbc.logger.Error(ctx, fmt.Errorf("error in DB operation: %w", err), logFields)
		} else if dbc.logging {
			dbc.logger.Debug(ctx, fmt.Sprintf("DB operation completed with resilience: %s", operationName), logFields)
		}

		return result, err
	}

	if dbc.logging {
		dbc.logger.Debug(ctx, fmt.Sprintf("starting DB operation: %s", operationName), logFields)
	}

	result, err := operation()

	if err != nil && dbc.logging {
		dbc.logger.Error(ctx, err, logFields)
	} else if dbc.logging {
		dbc.logger.Debug(ctx, fmt.Sprintf("DB operation completed: %s", operationName), logFields)
	}

	return result, err
}

func (dbc *DBClient) WithContext(ctx context.Context) *gorm.DB {
	return dbc.db.WithContext(ctx)
}

func (dbc *DBClient) Create(ctx context.Context, value interface{}) error {
	_, err := dbc.execute(ctx, "Create", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Create(value)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) First(ctx context.Context, dest interface{}, conditions ...interface{}) error {
	_, err := dbc.execute(ctx, "First", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).First(dest, conditions...)
		return nil, result.Error
	})

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func (dbc *DBClient) Find(ctx context.Context, dest interface{}, conditions ...interface{}) error {
	_, err := dbc.execute(ctx, "Find", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Find(dest, conditions...)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) Update(ctx context.Context, model interface{}, updates interface{}) error {
	_, err := dbc.execute(ctx, "Update", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Model(model).Updates(updates)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) Delete(ctx context.Context, value interface{}, conditions ...interface{}) error {
	_, err := dbc.execute(ctx, "Delete", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Delete(value, conditions...)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) Count(ctx context.Context, model interface{}, count *int64, conditions ...interface{}) error {
	_, err := dbc.execute(ctx, "Count", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Model(model)
		if len(conditions) > 0 {
			result = result.Where(conditions[0], conditions[1:]...)
		}
		result = result.Count(count)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) Exec(ctx context.Context, sql string, values ...interface{}) error {
	_, err := dbc.execute(ctx, "Exec", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Exec(sql, values...)
		return nil, result.Error
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
		result := dbc.db.WithContext(ctx).Preload(relation, conditions...).Find(dest)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) Where(ctx context.Context, dest interface{}, query interface{}, args ...interface{}) error {
	_, err := dbc.execute(ctx, "Where", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Where(query, args...).Find(dest)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) Order(ctx context.Context, dest interface{}, value interface{}) error {
	_, err := dbc.execute(ctx, "Order", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Order(value).Find(dest)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) Limit(ctx context.Context, dest interface{}, limit int) error {
	_, err := dbc.execute(ctx, "Limit", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Limit(limit).Find(dest)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) Offset(ctx context.Context, dest interface{}, offset int) error {
	_, err := dbc.execute(ctx, "Offset", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Offset(offset).Find(dest)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) Upsert(ctx context.Context, value interface{}, conflictColumns []string, updateColumns []string) error {
	columns := make([]clause.Column, len(conflictColumns))
	for i, col := range conflictColumns {
		columns[i] = clause.Column{Name: col}
	}

	_, err := dbc.execute(ctx, "Upsert", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   columns,
			DoUpdates: clause.AssignmentColumns(updateColumns),
		}).Create(value)
		return nil, result.Error
	})
	return err
}

func (dbc *DBClient) AutoMigrate(models ...interface{}) error {
	err := dbc.db.AutoMigrate(models...)
	if err != nil {
		return dbc.logger.WrapError(err, "error in auto migration")
	}
	return nil
}

func (dbc *DBClient) Raw(ctx context.Context, dest interface{}, sql string, values ...interface{}) error {
	_, err := dbc.execute(ctx, "Raw", func() (interface{}, error) {
		result := dbc.db.WithContext(ctx).Raw(sql, values...).Scan(dest)
		return nil, result.Error
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

type gormLogAdapter struct {
	logger   logger.Service
	logLevel string
}

func (l *gormLogAdapter) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return l
}

func (l *gormLogAdapter) Info(ctx context.Context, msg string, data ...interface{}) {
	logMsg := fmt.Sprintf(msg, data...)
	fields := map[string]interface{}{"type": "info"}
	l.logger.Info(ctx, logMsg, fields)
}

func (l *gormLogAdapter) Warn(ctx context.Context, msg string, data ...interface{}) {
	logMsg := fmt.Sprintf(msg, data...)
	fields := map[string]interface{}{"type": "warn"}
	l.logger.Warn(ctx, logMsg, fields)
}

func (l *gormLogAdapter) Error(ctx context.Context, msg string, data ...interface{}) {
	logMsg := fmt.Sprintf(msg, data...)
	fields := map[string]interface{}{"type": "error"}
	l.logger.Error(ctx, errors.New(logMsg), fields)
}

func (l *gormLogAdapter) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := map[string]interface{}{
		"elapsed": elapsed,
		"rows":    rows,
		"sql":     sql,
	}

	if err != nil {
		l.logger.Error(ctx, err, fields)
		return
	}

		l.logger.Debug(ctx, "SQL executed", fields)
}

// createGormLogger creates a gormlogger.Interface that adapts the provided logger.Service and log level for GORM.
// The returned adapter implements GORM's logging interface using the given logger and log level.
func createGormLogger(log logger.Service, logLevel string) gormlogger.Interface {
	return &gormLogAdapter{
		logger:   log,
		logLevel: logLevel,
	}
}