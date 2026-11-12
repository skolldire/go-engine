//go:build e2e

package test

import (
	"fmt"
	"testing"
	"time"

	redisprovider "github.com/skolldire/go-engine/database/redis/provider/redis"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestEngine_Redis exercises the provider against a real server: the connection
// the unit tests mock out, the key prefix applied to real keys, and the health
// check reporting real state.
func TestEngine_Redis(t *testing.T) {
	requireDocker(t)

	ctx := ctxWithTimeout(t, 3*time.Minute)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		},
		Started: true,
	})
	require.NoError(t, err)
	terminate(t, container)

	host, port := hostPort(t, container, "6379/tcp")

	dir := writeConfig(t, fmt.Sprintf(`
log:
  level: error
health:
  timeout: 5s
redis_clients:
  - cache:
      host: %q
      port: %d
      prefix: "e2e"
      timeout: 10s
`, host, port))

	eng := newEngine(t, dir,
		engine.WithHealth(),
		engine.WithProvider(redisprovider.New("cache")))

	client, err := redisprovider.From(eng, "cache")
	require.NoError(t, err, "the provider must expose a typed client")

	t.Run("round-trips a value", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, "greeting", "hola", time.Minute))

		got, err := client.Get(ctx, "greeting")
		require.NoError(t, err)
		assert.Equal(t, "hola", got)
	})

	t.Run("applies the configured key prefix", func(t *testing.T) {
		// The prefix is a real behaviour of the client, and a wrong one silently
		// reads another application's keys.
		assert.Equal(t, "e2e:greeting", client.KeyName("greeting"))
	})

	t.Run("reports a missing key distinctly", func(t *testing.T) {
		_, err := client.Get(ctx, "never-written")
		assert.Error(t, err, "a missing key must not read as an empty value")
	})

	t.Run("health reports ready against the live server", func(t *testing.T) {
		require.NotNil(t, eng.Health())
		assert.True(t, eng.Health().IsReady(ctx),
			"the provider registers a check; it must pass while the server is up")
	})

	t.Run("counters", func(t *testing.T) {
		n, err := client.Incr(ctx, "visits")
		require.NoError(t, err)
		assert.Equal(t, int64(1), n)

		n, err = client.IncrBy(ctx, "visits", 5)
		require.NoError(t, err)
		assert.Equal(t, int64(6), n, "IncrBy must accumulate on the same key")
	})

	t.Run("SetNX only sets an absent key", func(t *testing.T) {
		ok, err := client.SetNX(ctx, "lock", "held", time.Minute)
		require.NoError(t, err)
		assert.True(t, ok, "the first writer takes the lock")

		ok, err = client.SetNX(ctx, "lock", "other", time.Minute)
		require.NoError(t, err)
		assert.False(t, ok, "a second writer must not overwrite it")
	})

	t.Run("existence and deletion", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, "temp", "x", time.Minute))

		n, err := client.Exists(ctx, "temp")
		require.NoError(t, err)
		assert.Equal(t, int64(1), n)

		deleted, err := client.Del(ctx, "temp")
		require.NoError(t, err)
		assert.Equal(t, int64(1), deleted)

		n, err = client.Exists(ctx, "temp")
		require.NoError(t, err)
		assert.Zero(t, n)
	})

	t.Run("expiry is applied and readable", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, "session", "v", time.Hour))

		ok, err := client.Expire(ctx, "session", 30*time.Minute)
		require.NoError(t, err)
		assert.True(t, ok)

		ttl, err := client.TTL(ctx, "session")
		require.NoError(t, err)
		assert.Greater(t, ttl, 25*time.Minute)
		assert.LessOrEqual(t, ttl, 30*time.Minute)
	})

	t.Run("hashes", func(t *testing.T) {
		_, err := client.HSet(ctx, "user:1", "name", "ada", "age", "36")
		require.NoError(t, err)

		name, err := client.HGet(ctx, "user:1", "name")
		require.NoError(t, err)
		assert.Equal(t, "ada", name)

		all, err := client.HGetAll(ctx, "user:1")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"name": "ada", "age": "36"}, all)
	})

	t.Run("lists", func(t *testing.T) {
		_, err := client.LPush(ctx, "queue", "a", "b")
		require.NoError(t, err)

		items, err := client.LRange(ctx, "queue", 0, -1)
		require.NoError(t, err)
		assert.Len(t, items, 2)

		popped, err := client.RPop(ctx, "queue")
		require.NoError(t, err)
		assert.Equal(t, "a", popped, "RPop takes from the opposite end of LPush")
	})

	t.Run("sets", func(t *testing.T) {
		_, err := client.SAdd(ctx, "tags", "go", "redis")
		require.NoError(t, err)

		members, err := client.SMembers(ctx, "tags")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"go", "redis"}, members)

		found, err := client.SIsMember(ctx, "tags", "go")
		require.NoError(t, err)
		assert.True(t, found)

		card, err := client.SCard(ctx, "tags")
		require.NoError(t, err)
		assert.Equal(t, int64(2), card)
	})

	t.Run("sorted sets", func(t *testing.T) {
		_, err := client.ZAdd(ctx, "leaderboard", 10, "ada")
		require.NoError(t, err)

		score, err := client.ZScore(ctx, "leaderboard", "ada")
		require.NoError(t, err)
		assert.InDelta(t, 10.0, score, 0.001)

		members, err := client.ZRange(ctx, "leaderboard", 0, -1)
		require.NoError(t, err)
		assert.Equal(t, []string{"ada"}, members)
	})

	t.Run("health turns unready when the server goes away", func(t *testing.T) {
		require.NoError(t, container.Stop(ctx, nil))

		assert.Eventually(t, func() bool {
			return !eng.Health().IsReady(ctx)
		}, 30*time.Second, time.Second,
			"a stopped dependency must stop reporting ready")
	})
}
