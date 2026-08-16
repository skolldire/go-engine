//go:build integration

// Package redis integration tests run only with `go test -tags integration`
// and require a real Redis instance. They are excluded from the default,
// hermetic unit-test suite. Point them at a broker with REDIS_ADDR
// (default "127.0.0.1:6379").
package redis

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func integrationConfig(t *testing.T) Config {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err, "REDIS_ADDR must be host:port")
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return Config{
		Host:        host,
		Port:        port,
		DialTimeout: 2 * time.Second,
	}
}

func TestIntegration_SetGetDelete(t *testing.T) {
	cfg := integrationConfig(t)
	client, err := NewClient(cfg, &mockLogger{})
	require.NoError(t, err, "a real Redis must be reachable for integration tests")
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	key := "go-engine:integration:" + strconv.FormatInt(time.Now().UnixNano(), 10)

	require.NoError(t, client.Set(ctx, key, "value", time.Minute))

	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, "value", got)

	n, err := client.Del(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	_, err = client.Get(ctx, key)
	assert.ErrorIs(t, err, ErrKeyNotFound)
}
