// Package sql contributes a GORM-backed SQL client to an engine.
//
// Unlike every other adapter, the driver cannot come from configuration: GORM
// needs a gorm.Dialector, and resolving one from a string such as "postgres"
// would mean this package importing the Postgres, MySQL, SQLite and SQL Server
// dialects at once — so every consumer would link all four to use one. The
// caller supplies the dialector instead, and keeps paying only for its own
// driver.
//
//	import "gorm.io/driver/postgres"
//
//	engine.New(ctx, engine.WithProvider(
//	    sqlprovider.New("main", postgres.Open(dsn)),
//	))
package sql

import (
	"context"
	"fmt"

	"github.com/skolldire/go-engine/database/sql/pkg/database/gormsql"
	"github.com/skolldire/go-engine/pkg/engine"
	"gorm.io/gorm"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "sql_clients"

// Provider builds one named SQL client.
type Provider struct {
	instance  string
	dialector gorm.Dialector
	client    *gormsql.DBClient
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the SQL client declared under instance, using the
// given driver. The dialector carries the DSN, so the configuration section
// holds only pool and behaviour settings.
func New(instance string, dialector gorm.Dialector) *Provider {
	return &Provider{instance: instance, dialector: dialector}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "sql:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider. It also contributes a health check, so a
// database outage surfaces on /ready without the application wiring anything.
func (p *Provider) Init(_ context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	if p.dialector == nil {
		return nil, fmt.Errorf("sql:%s: no driver given; pass one such as postgres.Open(dsn)", p.instance)
	}

	cfg, err := engine.DecodeNamed[gormsql.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	client, err := gormsql.New(cfg, p.dialector, deps.Logger)
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

// From retrieves the SQL client registered under instance.
func From(e *engine.Engine, instance string) (*gormsql.DBClient, error) {
	return engine.Get[*gormsql.DBClient](e, "sql:"+instance)
}
