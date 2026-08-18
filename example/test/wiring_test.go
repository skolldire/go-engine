// Package test validates the example service against the library.
//
// The tests are split by what they need:
//
//   - This file and the layer tests need nothing: they run on every `go test`
//     and are the gate that the library still works when implemented.
//   - The files behind the `e2e` build tag need Docker, and prove each adapter
//     works against the real backing service.
package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skolldire/go-engine/database/redis/pkg/database/redis"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoConfigDir points at the service's real configuration directory, so these
// tests read the same file the deployed service does. A test with its own
// hand-written YAML would keep passing after the real one had drifted.
func repoConfigDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "config"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "application.yaml"))
	require.NoError(t, err, "the service configuration must exist")

	return dir
}

// TestServiceConfigurationIsValid is the cheapest guard that matters: the file
// the service ships with must actually load. A renamed key, a bare-number
// duration or a malformed section fails here rather than at deploy time.
func TestServiceConfigurationIsValid(t *testing.T) {
	eng, err := engine.New(context.Background(),
		engine.WithConfigDir(repoConfigDir(t)),
		engine.WithConfigFiles("application", "application-test"),
	)
	require.NoError(t, err, "config/application.yaml must load")
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	cfg := eng.Config()
	require.NotNil(t, cfg)

	t.Run("core sections are typed", func(t *testing.T) {
		assert.Equal(t, "0", cfg.Router.Port, "the test overlay must win")
		assert.Equal(t, "error", cfg.Log.Level)
		assert.Equal(t, 10*time.Second, cfg.Router.ReadTimeout,
			"durations come from the base file as real durations, not nanoseconds")
		assert.Equal(t, 5*time.Second, cfg.Health.Timeout)
	})

	t.Run("adapter sections survive untouched", func(t *testing.T) {
		// This is the property the whole decoupling rests on: the core does not
		// know these keys, and carries them through for their provider.
		for _, key := range []string{"redis_clients", "sql_clients", "rest"} {
			assert.True(t, cfg.Section(key).Exists(), "section %q must reach its provider", key)
		}
	})

	t.Run("declared instances are discoverable", func(t *testing.T) {
		names, err := engine.InstanceNames(cfg.Section("redis_clients"))
		require.NoError(t, err)
		assert.Equal(t, []string{"cache"}, names)
	})
}

// TestEngineBuildsWithoutAdapters proves the core is usable on its own: an HTTP
// service that registers no provider must still get a router and health probes.
func TestEngineBuildsWithoutAdapters(t *testing.T) {
	eng, err := engine.New(context.Background(),
		engine.WithConfigDir(repoConfigDir(t)),
		engine.WithConfigFiles("application", "application-test"),
		engine.WithRouter(),
		engine.WithHealth(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	assert.NotNil(t, eng.Router())
	assert.NotNil(t, eng.Health())
	assert.Empty(t, eng.ComponentNames(),
		"a section with no provider registered must build nothing")
}

// TestEnvironmentPlaceholdersResolve covers the ${VAR:-default} form the
// service's configuration uses for its hostnames.
func TestEnvironmentPlaceholdersResolve(t *testing.T) {
	t.Setenv("REDIS_HOST", "redis.internal")

	eng, err := engine.New(context.Background(),
		engine.WithConfigDir(repoConfigDir(t)),
		engine.WithConfigFiles("application", "application-test"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	// Decoding into the adapter's own Config is the honest check: it proves the
	// service's YAML matches the type the provider will actually use, including
	// that no key in the file is unknown to it.
	cfg, err := engine.DecodeNamed[redis.Config](eng.Config().Section("redis_clients"), "cache")
	require.NoError(t, err)

	assert.Equal(t, "redis.internal", cfg.Host, "${REDIS_HOST} must resolve from the environment")
	assert.Equal(t, 6379, cfg.Port)
	assert.Equal(t, "example", cfg.Prefix)
}

// TestEnvironmentPlaceholderFallsBackToDefault is the other half: an unset
// variable must use the declared default rather than an empty string.
func TestEnvironmentPlaceholderFallsBackToDefault(t *testing.T) {
	t.Setenv("REDIS_HOST", "")

	eng, err := engine.New(context.Background(),
		engine.WithConfigDir(repoConfigDir(t)),
		engine.WithConfigFiles("application", "application-test"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	cfg, err := engine.DecodeNamed[redis.Config](eng.Config().Section("redis_clients"), "cache")
	require.NoError(t, err)
	assert.Equal(t, "localhost", cfg.Host)
}

var _ = health.Config{}
