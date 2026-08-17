package grpc

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
)

// startServer starts a real gRPC server on an ephemeral port. A plain TCP
// listener is not enough: without the HTTP/2 handshake the connection never
// leaves CONNECTING, so READY would be unreachable.
func startServer(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(lis)
	}()

	t.Cleanup(func() {
		srv.Stop()
		<-done
	})

	return lis.Addr().String()
}

// freePort reserves an ephemeral port and releases it, yielding an address
// where nothing is listening.
func freePort(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())

	return addr
}

// TestNewClient_DoesNotBlockOnIdleConnection is the regression test for the bug
// where grpc.NewClient returned an IDLE connection and waitForConnection blocked
// on WaitForStateChange(ctx, IDLE) until the full timeout elapsed, failing even
// against a reachable target.
func TestNewClient_DoesNotBlockOnIdleConnection(t *testing.T) {
	target := startServer(t)

	start := time.Now()
	c, err := NewClient(Config{
		Target:  target,
		TimeOut: 3 * time.Second,
	}, &testutil.MockLogger{})
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, c)
	t.Cleanup(func() { _ = c.Close() })

	assert.Less(t, elapsed, time.Second,
		"NewClient must not block waiting for a connection it never asked to establish")
	assert.NotEqual(t, connectivity.Idle, c.CheckConnection(),
		"Connect() must have moved the connection out of IDLE")
}

// TestNewClient_WaitForReadyReachesReady verifies the opt-in blocking path still
// works: with WaitForReady the constructor waits until the connection is usable.
func TestNewClient_WaitForReadyReachesReady(t *testing.T) {
	target := startServer(t)

	c, err := NewClient(Config{
		Target:       target,
		WaitForReady: true,
		TimeOut:      5 * time.Second,
	}, &testutil.MockLogger{})

	require.NoError(t, err)
	require.NotNil(t, c)
	t.Cleanup(func() { _ = c.Close() })

	assert.Equal(t, connectivity.Ready, c.CheckConnection())
}

// TestNewClient_WaitForReadyUnreachableTargetFails ensures the blocking path
// still reports an error for a target that cannot be reached.
func TestNewClient_WaitForReadyUnreachableTargetFails(t *testing.T) {
	target := freePort(t)

	c, err := NewClient(Config{
		Target:       target,
		WaitForReady: true,
		TimeOut:      2 * time.Second,
	}, &testutil.MockLogger{})

	require.Error(t, err)
	assert.Nil(t, c)
}

// TestNewClient_LazyConnectionToUnreachableTargetSucceeds documents the default
// behaviour: without WaitForReady the constructor does not validate reachability,
// which is idiomatic for gRPC-Go.
func TestNewClient_LazyConnectionToUnreachableTargetSucceeds(t *testing.T) {
	target := freePort(t)

	start := time.Now()
	c, err := NewClient(Config{Target: target, TimeOut: 3 * time.Second}, &testutil.MockLogger{})
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, c)
	t.Cleanup(func() { _ = c.Close() })

	assert.Less(t, elapsed, time.Second)
}

// TestWithLogging_NoDataRace is the regression test for WithLogging mutating
// c.logging without the mutex that guards the rest of the client state, while
// execute/InvokeRPC read it concurrently.
func TestWithLogging_NoDataRace(t *testing.T) {
	target := startServer(t)

	c, err := NewClient(Config{
		Target:        target,
		EnableLogging: true,
		TimeOut:       3 * time.Second,
	}, &testutil.MockLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.WithLogging(i%2 == 0)
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.InvokeRPC(context.Background(), "op", func(ctx context.Context) (any, error) {
				return "ok", nil
			})
		}()
	}
	wg.Wait()
}

func TestClient_WithMetadataAndHeaders(t *testing.T) {
	target := startServer(t)
	c, err := NewClient(Config{Target: target, TimeOut: 3 * time.Second}, &testutil.MockLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	t.Run("WithMetadata attaches outgoing metadata", func(t *testing.T) {
		ctx := c.WithMetadata(context.Background(), metadata.Pairs("x-tenant", "acme"))
		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)
		assert.Equal(t, []string{"acme"}, md.Get("x-tenant"))
	})

	t.Run("WithHeaders attaches a header map", func(t *testing.T) {
		ctx := c.WithHeaders(context.Background(), map[string]string{"x-request-id": "req-1"})
		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)
		assert.Equal(t, []string{"req-1"}, md.Get("x-request-id"))
	})
}

func TestClient_GetConnection(t *testing.T) {
	target := startServer(t)
	c, err := NewClient(Config{Target: target, TimeOut: 3 * time.Second}, &testutil.MockLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	assert.NotNil(t, c.GetConnection())
	assert.Equal(t, c.GetConnection().GetState(), c.CheckConnection())
}

func TestClient_InvokeRPCPropagatesResultAndError(t *testing.T) {
	target := startServer(t)
	c, err := NewClient(Config{
		Target:        target,
		EnableLogging: true,
		TimeOut:       3 * time.Second,
	}, &testutil.MockLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	t.Run("result is returned", func(t *testing.T) {
		res, err := c.InvokeRPC(context.Background(), "ok", func(context.Context) (any, error) {
			return "value", nil
		})
		require.NoError(t, err)
		assert.Equal(t, "value", res)
	})

	t.Run("error is propagated", func(t *testing.T) {
		sentinel := errors.New("rpc failed")
		_, err := c.InvokeRPC(context.Background(), "bad", func(context.Context) (any, error) {
			return nil, sentinel
		})
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestClient_InvokeRPCWithResilience(t *testing.T) {
	target := startServer(t)
	c, err := NewClient(Config{
		Target:         target,
		WithResilience: true,
		EnableLogging:  true,
		TimeOut:        3 * time.Second,
	}, &testutil.MockLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	res, err := c.InvokeRPC(context.Background(), "resilient", func(context.Context) (any, error) {
		return 42, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 42, res)
}

func TestIsUsable(t *testing.T) {
	usable := []connectivity.State{connectivity.Ready, connectivity.Idle, connectivity.Connecting}
	for _, s := range usable {
		assert.True(t, isUsable(s), "%v must be usable", s)
	}

	notUsable := []connectivity.State{connectivity.TransientFailure, connectivity.Shutdown}
	for _, s := range notUsable {
		assert.False(t, isUsable(s), "%v must not be usable", s)
	}
}

func TestNewClient_DefaultsTimeout(t *testing.T) {
	target := startServer(t)
	c, err := NewClient(Config{Target: target}, &testutil.MockLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.NotNil(t, c)
}

// TestInvokeRPC_NudgesAnIdleConnection replaces the tests for the removed
// ReconnectIfNeeded.
//
// grpc.ClientConn reconnects on its own with its own backoff. The old manual
// reconnection dialled a replacement by hand: it duplicated that logic, could
// block while holding a write lock, and discarded the healthy subchannels the
// SDK was already re-establishing. All that is needed is to take an idle
// connection out of IDLE.
func TestInvokeRPC_NudgesAnIdleConnection(t *testing.T) {
	target := startServer(t)

	c, err := NewClient(Config{Target: target, TimeOut: 3 * time.Second}, &testutil.MockLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	before := c.GetConnection()

	res, err := c.InvokeRPC(context.Background(), "op", func(context.Context) (any, error) {
		return "ok", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", res)
	assert.Same(t, before, c.GetConnection(),
		"the connection must never be swapped out from under the caller")
}

// TestInvokeRPC_UnreachableTargetStillReturns guards that a downed upstream
// surfaces the operation's own error rather than hanging on a reconnect.
func TestInvokeRPC_UnreachableTargetStillReturns(t *testing.T) {
	target := freePort(t)

	c, err := NewClient(Config{Target: target, TimeOut: 2 * time.Second}, &testutil.MockLogger{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	sentinel := errors.New("rpc failed")
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err = c.InvokeRPC(context.Background(), "op", func(context.Context) (any, error) {
			return nil, sentinel
		})
	}()

	select {
	case <-done:
		assert.ErrorIs(t, err, sentinel)
	case <-time.After(5 * time.Second):
		t.Fatal("InvokeRPC must not block trying to reconnect")
	}
}
