package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
func (m *mockLogger) WrapError(err error, msg string) error                                    { return err }
func (m *mockLogger) WithField(key string, value interface{}) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                                      { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                           { return nil }

func TestGetRegistry(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry1 := GetRegistry()
	registry1.SetLogger(log)
	registry2 := GetRegistry()
	registry2.SetLogger(log)

	// Should return the same instance (singleton)
	assert.Equal(t, registry1, registry2)
	assert.NotNil(t, registry1)
}

func TestRegistry_Register(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry := GetRegistry()
	registry.SetLogger(log)

	clientName := "test-client-register"
	factory := func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error) {
		return "test-client", nil
	}

	err := registry.Register(clientName, factory)
	assert.NoError(t, err)
	assert.True(t, registry.IsRegistered(clientName))
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry := GetRegistry()
	registry.SetLogger(log)

	factory := func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error) {
		return "test-client", nil
	}

	clientName := "test-client-duplicate"
	err := registry.Register(clientName, factory)
	assert.NoError(t, err)

	// Try to register again
	err = registry.Register(clientName, factory)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_Create(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry := GetRegistry()
	registry.SetLogger(log)

	factory := func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error) {
		return "created-client", nil
	}

	clientName := "test-client-create"
	err := registry.Register(clientName, factory)
	assert.NoError(t, err)

	ctx := context.Background()
	client, err := registry.Create(ctx, clientName, nil)
	assert.NoError(t, err)
	assert.Equal(t, "created-client", client)
}

func TestRegistry_Create_NotRegistered(t *testing.T) {
	log := &mockLogger{}
	registry := GetRegistry()
	registry.SetLogger(log)

	ctx := context.Background()
	_, err := registry.Create(ctx, "nonexistent-client", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestRegistry_Create_WithError(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry := GetRegistry()
	registry.SetLogger(log)

	testErr := errors.New("factory error")
	factory := func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error) {
		return nil, testErr
	}

	clientName := "test-client-error"
	err := registry.Register(clientName, factory)
	assert.NoError(t, err)

	ctx := context.Background()
	_, err = registry.Create(ctx, clientName, nil)
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
}

func TestRegistry_IsRegistered(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry := GetRegistry()
	registry.SetLogger(log)

	clientName := "test-client-is-registered"
	assert.False(t, registry.IsRegistered(clientName))

	factory := func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error) {
		return "test-client", nil
	}

	err := registry.Register(clientName, factory)
	assert.NoError(t, err)
	assert.True(t, registry.IsRegistered(clientName))
}

func TestRegistry_ListRegistered(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry := GetRegistry()
	registry.SetLogger(log)

	factory := func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error) {
		return "client", nil
	}

	err := registry.Register("client1", factory)
	assert.NoError(t, err)

	err = registry.Register("client2", factory)
	assert.NoError(t, err)

	registered := registry.ListRegistered()
	assert.Contains(t, registered, "client1")
	assert.Contains(t, registered, "client2")
	assert.GreaterOrEqual(t, len(registered), 2)
}

func TestRegistry_Unregister(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry := GetRegistry()
	registry.SetLogger(log)

	clientName := "test-client-unregister"
	factory := func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error) {
		return "test-client", nil
	}

	err := registry.Register(clientName, factory)
	assert.NoError(t, err)
	assert.True(t, registry.IsRegistered(clientName))

	err = registry.Unregister(clientName)
	assert.NoError(t, err)
	assert.False(t, registry.IsRegistered(clientName))
}

func TestRegistry_Unregister_NotRegistered(t *testing.T) {
	log := &mockLogger{}
	registry := GetRegistry()
	registry.SetLogger(log)

	err := registry.Unregister("nonexistent-client")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry := GetRegistry()
	registry.SetLogger(log)

	factory := func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error) {
		return "client", nil
	}

	// Test concurrent registration
	done := make(chan bool, 2)
	go func() {
		_ = registry.Register("client1", factory)
		done <- true
	}()
	go func() {
		_ = registry.Register("client2", factory)
		done <- true
	}()

	<-done
	<-done

	assert.True(t, registry.IsRegistered("client1"))
	assert.True(t, registry.IsRegistered("client2"))
}

func TestRegistry_Create_WithConfig(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	registry := GetRegistry()
	registry.SetLogger(log)

	clientName := "test-client-config"
	config := map[string]string{"key": "value"}
	factory := func(ctx context.Context, cfg interface{}, log logger.Service) (interface{}, error) {
		assert.Equal(t, config, cfg)
		return "configured-client", nil
	}

	err := registry.Register(clientName, factory)
	assert.NoError(t, err)

	ctx := context.Background()
	client, err := registry.Create(ctx, clientName, config)
	assert.NoError(t, err)
	assert.Equal(t, "configured-client", client)
}
