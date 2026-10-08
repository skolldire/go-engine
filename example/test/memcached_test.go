//go:build e2e

package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/skolldire/go-engine/database/memcached/pkg/database/memcached"
	memcachedprovider "github.com/skolldire/go-engine/database/memcached/provider/memcached"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestEngine_Memcached round-trips a value through a real server.
func TestEngine_Memcached(t *testing.T) {
	requireDocker(t)

	ctx := ctxWithTimeout(t, 3*time.Minute)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "memcached:1.6-alpine",
			ExposedPorts: []string{"11211/tcp"},
			WaitingFor:   wait.ForListeningPort("11211/tcp").WithStartupTimeout(time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err)
	terminate(t, container)

	addr := hostAddr(t, container, "11211/tcp")

	dir := writeConfig(t, fmt.Sprintf(`
log:
  level: error
memcached_clients:
  - cache:
      servers:
        - %q
      timeout: 10s
`, addr))

	eng := newEngine(t, dir, engine.WithProvider(memcachedprovider.New("cache")))

	client, err := memcachedprovider.From(eng, "cache")
	require.NoError(t, err)

	t.Run("round-trips a value", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, "greeting", []byte("hola"), time.Minute))

		got, err := client.Get(ctx, "greeting")
		require.NoError(t, err)
		assert.Equal(t, []byte("hola"), got)
	})

	t.Run("reports a missing key distinctly", func(t *testing.T) {
		_, err := client.Get(ctx, "never-written")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKeyNotFound(),
			"a cache miss must be distinguishable from a transport failure")
	})

	t.Run("Add only writes an absent key", func(t *testing.T) {
		require.NoError(t, client.Add(ctx, "unique", []byte("first"), time.Minute))

		err := client.Add(ctx, "unique", []byte("second"), time.Minute)
		assert.Error(t, err, "Add must not overwrite an existing key")

		got, err := client.Get(ctx, "unique")
		require.NoError(t, err)
		assert.Equal(t, []byte("first"), got)
	})

	t.Run("Replace only writes an existing key", func(t *testing.T) {
		err := client.Replace(ctx, "absent", []byte("x"), time.Minute)
		assert.Error(t, err, "Replace must fail when the key is missing")

		require.NoError(t, client.Set(ctx, "present", []byte("old"), time.Minute))
		require.NoError(t, client.Replace(ctx, "present", []byte("new"), time.Minute))

		got, err := client.Get(ctx, "present")
		require.NoError(t, err)
		assert.Equal(t, []byte("new"), got)
	})

	t.Run("counters", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, "hits", []byte("10"), time.Minute))

		n, err := client.Increment(ctx, "hits", 5)
		require.NoError(t, err)
		assert.Equal(t, uint64(15), n)

		n, err = client.Decrement(ctx, "hits", 3)
		require.NoError(t, err)
		assert.Equal(t, uint64(12), n)
	})

	t.Run("GetMulti returns only the keys that exist", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, "m1", []byte("a"), time.Minute))
		require.NoError(t, client.Set(ctx, "m2", []byte("b"), time.Minute))

		got, err := client.GetMulti(ctx, []string{"m1", "m2", "missing"})
		require.NoError(t, err)
		assert.Len(t, got, 2, "an absent key is omitted, not an error")
	})

	t.Run("Delete removes a key", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, "doomed", []byte("x"), time.Minute))
		require.NoError(t, client.Delete(ctx, "doomed"))

		_, err := client.Get(ctx, "doomed")
		assert.Error(t, err)
	})

	t.Run("FlushAll empties the server", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, "survivor", []byte("x"), time.Minute))
		require.NoError(t, client.FlushAll(ctx))

		_, err := client.Get(ctx, "survivor")
		assert.Error(t, err)
	})
}

// ErrKeyNotFound exposes the adapter's sentinel to the test without importing
// the package under a name that would shadow the provider alias.
func ErrKeyNotFound() error { return memcached.ErrKeyNotFound }
