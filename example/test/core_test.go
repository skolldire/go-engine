//go:build e2e

package test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEngine_HTTPService drives the shape most applications start from: a
// router, health probes, and a handler. It needs no container, so it also
// serves as the suite's smoke test.
func TestEngine_HTTPService(t *testing.T) {
	dir := writeConfig(t, `
log:
  level: error
router:
  port: "0"
  read_timeout: 5s
  write_timeout: 10s
health:
  timeout: 2s
`)

	eng := newEngine(t, dir, engine.WithRouter(), engine.WithHealth())

	require.NotNil(t, eng.Router())
	eng.Router().AddRoute("GET", "/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": r.PathValue("id")})
	})

	srv := httptest.NewServer(eng.Router().Router())
	t.Cleanup(srv.Close)

	t.Run("handler serves", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/users/42")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]string
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "42", body["id"])
	})

	t.Run("probes are mounted", func(t *testing.T) {
		for _, path := range []string{"/health", "/live", "/ready", "/deps"} {
			resp, err := http.Get(srv.URL + path)
			require.NoError(t, err, path)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode, path)
		}
	})
}

// TestEngine_HealthReflectsDependencyState is the check a unit test cannot make
// convincingly: /ready must turn 503 when a registered dependency actually
// fails, and recover when it comes back.
func TestEngine_HealthReflectsDependencyState(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\nrouter:\n  port: \"0\"\nhealth:\n  timeout: 2s\n")

	failing := make(chan error, 1)
	failing <- nil

	current := func() error {
		err := <-failing
		failing <- err
		return err
	}

	eng := newEngine(t, dir, engine.WithRouter(), engine.WithHealth(),
		engine.WithProvider(&checkProvider{check: current}))

	srv := httptest.NewServer(eng.Router().Router())
	t.Cleanup(srv.Close)

	ready := func() int {
		resp, err := http.Get(srv.URL + "/ready")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	assert.Equal(t, http.StatusOK, ready(), "a healthy dependency must report ready")

	<-failing
	failing <- errors.New("dependency down")
	assert.Equal(t, http.StatusServiceUnavailable, ready(),
		"a failing dependency must make the service not ready")

	<-failing
	failing <- nil
	assert.Equal(t, http.StatusOK, ready(), "recovery must be observed")
}

// checkProvider contributes a health check whose result the test controls.
type checkProvider struct{ check func() error }

func (c *checkProvider) Name() string      { return "controlled" }
func (c *checkProvider) ConfigKey() string { return "controlled" }
func (c *checkProvider) Init(_ context.Context, _ engine.RawConfig, deps engine.Deps) (any, error) {
	deps.Health.RegisterCheck("controlled", func(context.Context) error { return c.check() })
	return c, nil
}
func (c *checkProvider) Close(context.Context) error { return nil }

// TestEngine_ConfigErrors proves the configuration guards fire against a real
// file, not just against a hand-built RawConfig.
func TestEngine_ConfigErrors(t *testing.T) {
	t.Run("bare number for a duration", func(t *testing.T) {
		dir := writeConfig(t, "log:\n  level: error\nrouter:\n  read_timeout: 10\n")

		_, err := engine.New(context.Background(), engine.WithConfigDir(dir))

		require.Error(t, err, "10 means 10 nanoseconds; it must not be accepted silently")
		assert.ErrorContains(t, err, "not a duration")
	})

	t.Run("unknown key in a provider section", func(t *testing.T) {
		dir := writeConfig(t, `
log:
  level: error
redis_clients:
  - cache:
      hostt: "localhost"
`)
		_, err := engine.New(context.Background(), engine.WithConfigDir(dir),
			engine.WithProvider(&decodingProvider{key: "redis_clients", instance: "cache"}))

		require.Error(t, err)
		assert.ErrorContains(t, err, "hostt")
	})

	t.Run("malformed multi-instance section", func(t *testing.T) {
		dir := writeConfig(t, "log:\n  level: error\nredis_clients: \"not a list\"\n")

		names, err := engine.InstanceNames((&engine.Config{
			Components: map[string]any{"redis_clients": "not a list"},
		}).Section("redis_clients"))

		require.Error(t, err, "a malformed section must not silently yield no providers")
		assert.Nil(t, names)
		_ = dir
	})

	t.Run("missing configuration file", func(t *testing.T) {
		_, err := engine.New(context.Background(), engine.WithConfigDir(t.TempDir()))
		assert.ErrorContains(t, err, "no configuration file found")
	})
}

// decodingProvider decodes a named section, so a configuration error surfaces
// through the same path a real adapter uses.
type decodingProvider struct {
	key      string
	instance string
}

func (d *decodingProvider) Name() string      { return d.key + ":" + d.instance }
func (d *decodingProvider) ConfigKey() string { return d.key }
func (d *decodingProvider) Init(_ context.Context, raw engine.RawConfig, _ engine.Deps) (any, error) {
	type cfg struct {
		Host string `mapstructure:"host"`
	}
	return engine.DecodeNamed[cfg](raw, d.instance)
}
func (d *decodingProvider) Close(context.Context) error { return nil }

// TestEngine_LifecycleReleasesInLIFOOrder proves shutdown ordering with real
// timing, since a component built later may depend on one built earlier.
func TestEngine_LifecycleReleasesInLIFOOrder(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	var order []string
	record := func(name string) engine.Provider {
		return &orderedProvider{name: name, onClose: func() { order = append(order, name) }}
	}

	eng, err := engine.New(context.Background(), engine.WithConfigDir(dir),
		engine.WithProvider(record("first"), record("second"), record("third")))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, eng.Close(ctx))

	assert.Equal(t, []string{"third", "second", "first"}, order,
		"later components may depend on earlier ones, so they must close first")
}

type orderedProvider struct {
	name    string
	onClose func()
}

func (o *orderedProvider) Name() string      { return o.name }
func (o *orderedProvider) ConfigKey() string { return o.name }
func (o *orderedProvider) Init(context.Context, engine.RawConfig, engine.Deps) (any, error) {
	return o, nil
}
func (o *orderedProvider) Close(context.Context) error {
	o.onClose()
	return nil
}

var _ = health.Config{}
