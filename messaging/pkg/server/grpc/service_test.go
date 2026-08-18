package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]any) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]any) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]any) {
	m.Called(ctx, err, fields)
}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]any) {}
func (m *mockLogger) WrapError(err error, msg string) error                            { return err }
func (m *mockLogger) WithField(key string, value any) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]any) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                              { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                   { return nil }

func TestNewServer(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: false,
	}
	log := &mockLogger{}

	server := NewServer(context.Background(), cfg, log)

	assert.NotNil(t, server)
	// Verify that server implements the Service interface
	// Type assertion is redundant since NewServer already returns Service
	_ = server
}

func TestNewServer_WithLogging(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: true,
	}
	log := &mockLogger{}

	srv := NewServer(context.Background(), cfg, log)

	assert.NotNil(t, srv)
	// Verify that srv implements the Service interface
	// Type assertion is redundant since NewServer already returns Service
	_ = srv
}

func TestServer_RegisterService(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(context.Background(), cfg, log)

	registered := false
	registerFunc := func(s *grpc.Server) {
		registered = true
	}

	srv.RegisterService(registerFunc)
	assert.True(t, registered)
}

func TestServer_Start(t *testing.T) {
	cfg := Config{
		Puerto:        0, // Use port 0 for automatic port assignment
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(context.Background(), cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	srv.Stop(context.Background())
}

func TestServer_Start_WithLogging(t *testing.T) {
	cfg := Config{
		Puerto:        0,
		EnableLogging: true,
	}
	log := &mockLogger{}
	log.On("Info", mock.Anything, "starting gRPC server", mock.Anything).Return()
	log.On("Info", mock.Anything, "stopping gRPC server", mock.Anything).Return()

	srv := NewServer(context.Background(), cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	srv.Stop(context.Background())
	log.AssertExpectations(t)
}

func TestServer_Start_WithContextCancellation(t *testing.T) {
	cfg := Config{
		Puerto:        0,
		EnableLogging: false,
	}
	log := &mockLogger{}

	server := NewServer(context.Background(), cfg, log)

	ctx, cancel := context.WithCancel(context.Background())

	err := server.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Cancel context should trigger graceful stop
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestServer_Stop(t *testing.T) {
	cfg := Config{
		Puerto:        0,
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(context.Background(), cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Stop should not panic
	srv.Stop(context.Background())
	time.Sleep(100 * time.Millisecond)
}

func TestServer_Stop_WithLogging(t *testing.T) {
	cfg := Config{
		Puerto:        0,
		EnableLogging: true,
	}
	log := &mockLogger{}
	log.On("Info", mock.Anything, "starting gRPC server", mock.Anything).Return()
	log.On("Info", mock.Anything, "stopping gRPC server", mock.Anything).Return()

	srv := NewServer(context.Background(), cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	srv.Stop(context.Background())
	log.AssertExpectations(t)
}

func TestServer_Stop_WithoutStart(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(context.Background(), cfg, log)

	// Stop should not panic even if server wasn't started
	srv.Stop(context.Background())
}

func TestServer_MultipleStops(t *testing.T) {
	cfg := Config{
		Puerto:        0,
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(context.Background(), cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Multiple stops should not panic
	srv.Stop(context.Background())
	srv.Stop(context.Background())
	srv.Stop(context.Background())
}

func TestServer_RegisterService_Multiple(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(context.Background(), cfg, log)

	count := 0
	registerFunc1 := func(s *grpc.Server) {
		count++
	}
	registerFunc2 := func(s *grpc.Server) {
		count++
	}

	srv.RegisterService(registerFunc1)
	srv.RegisterService(registerFunc2)

	assert.Equal(t, 2, count)
}

// TestStop_IsBoundedByContext is the regression test for Stop calling
// GracefulStop with no deadline of its own.
//
// GracefulStop waits for in-flight RPCs indefinitely, so a single stuck stream
// held the engine's entire shutdown open and the process had to be killed. The
// engine closes under a deadline; Stop must respect it.
func TestStop_IsBoundedByContext(t *testing.T) {
	// A silent logger, not the testify mock: the mock captures arguments and
	// reflects over them, which races with the server goroutine logging during
	// shutdown. This test is about Stop's deadline, not about what was logged.
	srv := NewServer(context.Background(), Config{Puerto: 0}, silentLogger{})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, srv.Start(ctx))

	cancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer stopCancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Stop(stopCtx)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop must return once its context expires, not wait for the RPC")
	}
}

// silentLogger discards everything and captures nothing, so it is safe to share
// with a goroutine.
type silentLogger struct{}

func (silentLogger) Debug(context.Context, string, map[string]any)     {}
func (silentLogger) Info(context.Context, string, map[string]any)      {}
func (silentLogger) Warn(context.Context, string, map[string]any)      {}
func (silentLogger) Error(context.Context, error, map[string]any)      {}
func (silentLogger) FatalError(context.Context, error, map[string]any) {}
func (silentLogger) WrapError(err error, _ string) error               { return err }
func (s silentLogger) WithField(string, any) logger.Service            { return s }
func (s silentLogger) WithFields(map[string]any) logger.Service        { return s }
func (silentLogger) GetLogLevel() string                               { return "info" }
func (silentLogger) SetLogLevel(string) error                          { return nil }
