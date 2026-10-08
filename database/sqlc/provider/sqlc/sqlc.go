// Package sqlc contributes a database/sql client to an engine, sized for
// sqlc-generated code.
//
// It is the light half of the SQL story. provider/sql builds a GORM client and
// needs a gorm.Dialector injected in Go, because a dialect is a value no YAML
// file can name; here the driver is a string that a blank import registers, so
// the whole component is declared in configuration:
//
//	import _ "github.com/jackc/pgx/v5/stdlib"
//
//	engine.New(ctx, engine.WithProvider(sqlc.New("main")))
//
// The two providers are independent modules with different configuration
// sections, so an application picks one, the other, or both — a service can
// keep a GORM client for its legacy tables while new queries go through sqlc.
package sqlc

import (
	"context"
	"database/sql/driver"
	"fmt"

	"github.com/skolldire/go-engine/database/sqlc/pkg/database/sqlcdb"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "sqlc_clients"

// Provider builds one named database/sql client.
type Provider struct {
	instance  string
	connector driver.Connector
	client    *sqlcdb.Client
}

var _ engine.Provider = (*Provider)(nil)

// Option configures the provider at construction.
type Option func(*Provider)

// WithConnector builds the pool from a driver.Connector instead of the
// configured DSN, for credentials that cannot live in a connection string —
// an IAM token, a secret rotated at runtime. The `dsn` key is then ignored.
func WithConnector(c driver.Connector) Option {
	return func(p *Provider) { p.connector = c }
}

// New returns a provider for the client declared under instance.
func New(instance string, opts ...Option) *Provider {
	p := &Provider{instance: instance}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "sqlc:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider. It also contributes a health check, so a
// database outage surfaces on /ready without the application wiring anything.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[sqlcdb.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	var client *sqlcdb.Client
	switch {
	case p.connector != nil:
		client, err = sqlcdb.NewWithConnector(ctx, cfg, p.connector, deps.Logger)
	case cfg.Driver == "":
		// The driver is a registration, not a value: a name with no blank
		// import behind it fails inside database/sql with "unknown driver",
		// which does not say what the application forgot to do.
		return nil, fmt.Errorf(
			"sqlc:%s: no driver configured; set `driver` in %s and blank-import it "+
				`(e.g. _ "github.com/jackc/pgx/v5/stdlib"), or pass sqlc.WithConnector(...)`,
			p.instance, ConfigKey)
	default:
		client, err = sqlcdb.New(ctx, cfg, deps.Logger)
	}
	if err != nil {
		return nil, err
	}
	p.client = client

	deps.Health.RegisterCheck(p.Name(), func(ctx context.Context) error {
		return client.Ping(ctx)
	})

	return client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error {
	if p.client == nil {
		return nil
	}
	return p.client.Close()
}

// From retrieves the client registered under instance.
func From(e *engine.Engine, instance string) (*sqlcdb.Client, error) {
	return engine.Get[*sqlcdb.Client](e, "sqlc:"+instance)
}
