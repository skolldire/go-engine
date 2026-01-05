package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]interface{}) {
	m.Called(ctx, err, fields)
}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {}
func (m *mockLogger) WrapError(err error, msg string) error { return err }
func (m *mockLogger) WithField(key string, value interface{}) logger.Service { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service { return m }
func (m *mockLogger) GetLogLevel() string { return "info" }
func (m *mockLogger) SetLogLevel(level string) error { return nil }

func TestNewServer(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: false,
	}
	log := &mockLogger{}

	server := NewServer(cfg, log)

	assert.NotNil(t, server)
	// Verificar que implementa la interfaz Service
	_, ok := server.(Service)
	assert.True(t, ok)
}

func TestNewServer_WithLogging(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: true,
	}
	log := &mockLogger{}

	srv := NewServer(cfg, log)

	assert.NotNil(t, srv)
	// Verificar que implementa la interfaz Service
	_, ok := srv.(Service)
	assert.True(t, ok)
}

func TestServer_RegisterService(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(cfg, log)

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

	srv := NewServer(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	srv.Stop()
}

func TestServer_Start_WithLogging(t *testing.T) {
	cfg := Config{
		Puerto:        0,
		EnableLogging: true,
	}
	log := &mockLogger{}
	log.On("Info", mock.Anything, "Iniciando servidor gRPC", mock.Anything).Return()

	srv := NewServer(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	srv.Stop()
	log.AssertExpectations(t)
}

func TestServer_Start_WithContextCancellation(t *testing.T) {
	cfg := Config{
		Puerto:        0,
		EnableLogging: false,
	}
	log := &mockLogger{}

	server := NewServer(cfg, log)

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

	srv := NewServer(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Stop should not panic
	srv.Stop()
	time.Sleep(100 * time.Millisecond)
}

func TestServer_Stop_WithLogging(t *testing.T) {
	cfg := Config{
		Puerto:        0,
		EnableLogging: true,
	}
	log := &mockLogger{}
	log.On("Info", mock.Anything, "Deteniendo servidor gRPC", mock.Anything).Return()

	srv := NewServer(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	srv.Stop()
	log.AssertExpectations(t)
}

func TestServer_Stop_WithoutStart(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(cfg, log)

	// Stop should not panic even if server wasn't started
	srv.Stop()
}

func TestServer_MultipleStops(t *testing.T) {
	cfg := Config{
		Puerto:        0,
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start(ctx)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Multiple stops should not panic
	srv.Stop()
	srv.Stop()
	srv.Stop()
}

func TestServer_RegisterService_Multiple(t *testing.T) {
	cfg := Config{
		Puerto:        50051,
		EnableLogging: false,
	}
	log := &mockLogger{}

	srv := NewServer(cfg, log)

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
