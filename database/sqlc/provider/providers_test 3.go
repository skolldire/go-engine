// Package provider_test verifies the sqlc provider contract.
package provider_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	sqlcprovider "github.com/skolldire/go-engine/database/sqlc/provider/sqlc"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deps() (engine.Deps, *stubHealth) {
	health := &stubHealth{}
	return engine.Deps{
		Logger:    logger.NewService(logger.Config{Level: "error"}, nil),
		Telemetry: stubTelemetry{},
		Health:    health,
		Section:   func(k string) engine.RawConfig { return engine.NewMissingRawConfig(k) },
		Resource:  func(_ string, b func() (any, error)) (any, error) { return b() },
	}, health
}

func section(key, instance string, cfg map[string]any) engine.RawConfig {
	return engine.NewRawConfig(key, []any{map[string]any{instance: cfg}})
}

func sqliteConfig() map[string]any {
	return map[string]any{"driver": "sqlite3", "dsn": ":memory:"}
}

func TestConfigKeyAndName(t *testing.T) {
	p := sqlcprovider.New("main")
	assert.Equal(t, "sqlc_clients", p.ConfigKey())
	assert.True(t, strings.HasSuffix(p.Name(), ":main"), "got %q", p.Name())
	assert.Equal(t, p.Name(), p.Name(), "Name must be stable across calls")
}

// TestNameDoesNotCollideWithTheGormProvider: both SQL providers can be
// registered in the same engine, so their component names must differ.
func TestNameDoesNotCollideWithTheGormProvider(t *testing.T) {
	assert.Equal(t, "sqlc:main", sqlcprovider.New("main").Name())
}

// TestBuildsFromConfiguration is the point of this provider's shape: unlike the
// GORM one, nothing has to be injected in Go — the driver is a registered name.
func TestBuildsFromConfiguration(t *testing.T) {
	p := sqlcprovider.New("main")
	raw := section(p.ConfigKey(), "main", map[string]any{
		"driver":               "sqlite3",
		"dsn":                  ":memory:",
		"max_open_connections": 4,
		"conn_max_lifetime":    "30s",
		"conn_max_idle_time":   "10s",
		"timeout":              "5s",
	})
	d, health := deps()

	built, err := p.Init(context.Background(), raw, d)

	require.NoError(t, err)
	assert.NotNil(t, built)
	assert.Equal(t, []string{"sqlc:main"}, health.registered, "the provider must contribute a health check")
	assert.NoError(t, p.Close(context.Background()))
}

func TestHealthCheckPingsTheDatabase(t *testing.T) {
	p := sqlcprovider.New("main")
	d, health := deps()
	require.NoError(t, initErr(p.Init(context.Background(), section(p.ConfigKey(), "main", sqliteConfig()), d)))
	t.Cleanup(func() { _ = p.Close(context.Background()) })

	require.Len(t, health.checks, 1)
	assert.NoError(t, health.checks[0](context.Background()))
}

// TestNoDriverIsReported: a name with no blank import behind it fails deep
// inside database/sql, so the provider says what the application forgot.
func TestNoDriverIsReported(t *testing.T) {
	p := sqlcprovider.New("main")
	raw := section(p.ConfigKey(), "main", map[string]any{"dsn": ":memory:"})
	d, _ := deps()

	_, err := p.Init(context.Background(), raw, d)

	require.Error(t, err)
	assert.ErrorContains(t, err, "no driver configured")
}

func TestWithConnectorBypassesTheDSN(t *testing.T) {
	connector, err := newSQLiteConnector()
	require.NoError(t, err)

	p := sqlcprovider.New("main", sqlcprovider.WithConnector(connector))
	raw := section(p.ConfigKey(), "main", map[string]any{})
	d, _ := deps()

	built, err := p.Init(context.Background(), raw, d)

	require.NoError(t, err)
	assert.NotNil(t, built)
	assert.NoError(t, p.Close(context.Background()))
}

func TestMissingSectionIsReported(t *testing.T) {
	p := sqlcprovider.New("main")
	d, _ := deps()
	_, err := p.Init(context.Background(), engine.NewMissingRawConfig(p.ConfigKey()), d)
	assert.Error(t, err)
}

func TestMissingInstanceIsReported(t *testing.T) {
	p := sqlcprovider.New("not-declared")
	raw := section(p.ConfigKey(), "other", sqliteConfig())
	d, _ := deps()

	_, err := p.Init(context.Background(), raw, d)

	require.Error(t, err)
	assert.ErrorContains(t, err, "not declared")
}

func TestUnknownKeyIsReported(t *testing.T) {
	p := sqlcprovider.New("main")
	raw := section(p.ConfigKey(), "main", map[string]any{"drivr": "sqlite3"})
	d, _ := deps()

	_, err := p.Init(context.Background(), raw, d)

	require.Error(t, err)
	assert.ErrorContains(t, err, "drivr")
}

// TestBareNumberDurationIsReported covers the footgun end to end for this family.
func TestBareNumberDurationIsReported(t *testing.T) {
	p := sqlcprovider.New("main")
	raw := section(p.ConfigKey(), "main", map[string]any{"conn_max_lifetime": 30})
	d, _ := deps()

	_, err := p.Init(context.Background(), raw, d)

	require.Error(t, err)
	assert.ErrorContains(t, err, "not a duration")
}

func TestCloseBeforeInitIsSafe(t *testing.T) {
	p := sqlcprovider.New("main")
	assert.NotPanics(t, func() { assert.NoError(t, p.Close(context.Background())) })
}

func initErr(_ any, err error) error { return err }

// dsnConnector is the shape a caller supplies when the credentials cannot live
// in a connection string. It reaches the driver through (*sql.DB).Driver()
// rather than the driver package's own types, so the test stays independent of
// the driver's internals.
type dsnConnector struct {
	dsn string
	drv driver.Driver
}

func (c dsnConnector) Connect(context.Context) (driver.Conn, error) { return c.drv.Open(c.dsn) }
func (c dsnConnector) Driver() driver.Driver                        { return c.drv }

func newSQLiteConnector() (driver.Connector, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	return dsnConnector{dsn: ":memory:", drv: db.Driver()}, nil
}

type stubTelemetry struct{}

func (stubTelemetry) Span(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}
func (stubTelemetry) Counter(context.Context, string, int64)     {}
func (stubTelemetry) Histogram(context.Context, string, float64) {}

type stubHealth struct {
	registered []string
	checks     []func(context.Context) error
}

func (s *stubHealth) RegisterCheck(name string, check func(context.Context) error) {
	s.registered = append(s.registered, name)
	s.checks = append(s.checks, check)
}
