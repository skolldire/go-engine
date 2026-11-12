//go:build e2e

package test

import (
	"fmt"
	"testing"
	"time"

	sqlprovider "github.com/skolldire/go-engine/database/sql/provider/sql"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
)

// TestEngine_Postgres validates the SQL provider, which is the only one whose
// driver is injected rather than read from configuration. That shape is
// untestable without a real database: a wrong DSN or a driver mismatch produces
// a client that builds and fails on first use.
func TestEngine_Postgres(t *testing.T) {
	requireDocker(t)

	ctx := ctxWithTimeout(t, 3*time.Minute)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "engine",
				"POSTGRES_PASSWORD": "secret",
				"POSTGRES_DB":       "engine_test",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err)
	terminate(t, container)

	host, port := hostPort(t, container, "5432/tcp")
	dsn := fmt.Sprintf("host=%s port=%d user=engine password=secret dbname=engine_test sslmode=disable",
		host, port)

	dir := writeConfig(t, `
log:
  level: error
health:
  timeout: 5s
sql_clients:
  - main:
      type: "postgres"
      max_open_connections: 5
      max_idle_connections: 2
      conn_max_lifetime: 5m
`)

	eng := newEngine(t, dir,
		engine.WithHealth(),
		engine.WithProvider(sqlprovider.New("main", postgres.Open(dsn))))

	db, err := sqlprovider.From(eng, "main")
	require.NoError(t, err)

	t.Run("executes real queries", func(t *testing.T) {
		require.NoError(t, db.DB().Exec(`CREATE TABLE IF NOT EXISTS users (id serial PRIMARY KEY, name text)`).Error)
		require.NoError(t, db.DB().Exec(`INSERT INTO users (name) VALUES (?)`, "ada").Error)

		var name string
		require.NoError(t, db.DB().Raw(`SELECT name FROM users LIMIT 1`).Scan(&name).Error)
		assert.Equal(t, "ada", name)
	})

	t.Run("honours the configured pool settings", func(t *testing.T) {
		sqlDB, err := db.DB().DB()
		require.NoError(t, err)

		// A pool size read from the wrong key silently leaves the driver default,
		// which only shows up as saturation under load.
		assert.Equal(t, 5, sqlDB.Stats().MaxOpenConnections)
	})

	t.Run("health reports ready against the live database", func(t *testing.T) {
		assert.True(t, eng.Health().IsReady(ctx))
	})
}
