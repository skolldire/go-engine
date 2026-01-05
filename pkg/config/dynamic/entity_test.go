package dynamic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockLogger is now defined in mocks_test.go

type mockWatcher struct {
	mock.Mock
}

func (m *mockWatcher) Watch(ctx context.Context, onChange func() error) error {
	args := m.Called(ctx, onChange)
	return args.Error(0)
}

func (m *mockWatcher) Stop() error {
	args := m.Called()
	return args.Error(0)
}

func TestNewDynamicConfig(t *testing.T) {
	initialConfig := map[string]string{"key": "value"}
	dc := NewDynamicConfig(initialConfig, nil)
	assert.NotNil(t, dc)
	assert.Equal(t, initialConfig, dc.Get())
}

func TestDynamicConfig_Get(t *testing.T) {
	initialConfig := map[string]string{"key": "value"}
	dc := NewDynamicConfig(initialConfig, nil)
	result := dc.Get()
	assert.Equal(t, initialConfig, result)
}

func TestDynamicConfig_SetReloadFunc(t *testing.T) {
	// Use empty map instead of nil to avoid atomic.Value panic
	dc := NewDynamicConfig(map[string]interface{}{}, nil)
	dc.SetReloadFunc(func() (interface{}, error) {
		return map[string]string{"new": "config"}, nil
	})
	assert.NotNil(t, dc)
}

func TestDynamicConfig_Reload_Success(t *testing.T) {
	mockLog := new(mockLogger)
	initialConfig := map[string]string{"key": "value"}
	dc := NewDynamicConfig(initialConfig, mockLog)

	newConfig := map[string]string{"new": "config"}
	dc.SetReloadFunc(func() (interface{}, error) {
		return newConfig, nil
	})

	mockLog.On("Info", context.Background(), "configuration reloaded successfully", mock.Anything).Return()

	err := dc.Reload()
	assert.NoError(t, err)
	assert.Equal(t, newConfig, dc.Get())
	mockLog.AssertExpectations(t)
}

func TestDynamicConfig_Reload_Error(t *testing.T) {
	mockLog := new(mockLogger)
	dc := NewDynamicConfig(map[string]interface{}{}, mockLog)

	testErr := errors.New("reload error")
	dc.SetReloadFunc(func() (interface{}, error) {
		return nil, testErr
	})

	mockLog.On("Error", context.Background(), testErr, mock.Anything).Return()

	err := dc.Reload()
	assert.Error(t, err)
	mockLog.AssertExpectations(t)
}

func TestDynamicConfig_Reload_NilFunc(t *testing.T) {
	dc := NewDynamicConfig(map[string]interface{}{}, nil)
	err := dc.Reload()
	assert.NoError(t, err) // Should return nil if no reload func
}

func TestDynamicConfig_AddWatcher(t *testing.T) {
	dc := NewDynamicConfig(map[string]interface{}{}, nil)
	watcher := new(mockWatcher)
	dc.AddWatcher(watcher)
	assert.NotNil(t, dc)
}

func TestDynamicConfig_AddReloadHook(t *testing.T) {
	dc := NewDynamicConfig(map[string]interface{}{}, nil)
	hook := func(oldConfig, newConfig interface{}) error {
		return nil
	}
	dc.AddReloadHook(hook)
	assert.NotNil(t, dc)
}

func TestDynamicConfig_StartWatching(t *testing.T) {
	mockLog := new(mockLogger)
	dc := NewDynamicConfig(map[string]interface{}{}, mockLog)
	watcher := new(mockWatcher)
	dc.AddWatcher(watcher)

	// Use mock.MatchedBy to match the function type
	// Note: Watch is called in a goroutine, so we use mock.MatchedBy
	watcher.On("Watch", mock.Anything, mock.MatchedBy(func(fn func() error) bool {
		return fn != nil
	})).Return(nil).Maybe() // Use Maybe() since it's called in a goroutine

	ctx := context.Background()
	err := dc.StartWatching(ctx)
	assert.NoError(t, err)
	// Give goroutine time to execute
	time.Sleep(10 * time.Millisecond)
	watcher.AssertExpectations(t)
}

func TestDynamicConfig_Stop(t *testing.T) {
	dc := NewDynamicConfig(map[string]interface{}{}, nil)
	watcher := new(mockWatcher)
	watcher.On("Stop").Return(nil)
	dc.AddWatcher(watcher)

	err := dc.Stop()
	assert.NoError(t, err)
	watcher.AssertExpectations(t)
}

func TestDynamicConfig_GetLastReload(t *testing.T) {
	dc := NewDynamicConfig(map[string]interface{}{}, nil)
	lastReload := dc.GetLastReload()
	assert.False(t, lastReload.IsZero())
}

func TestDynamicConfig_ValidateConfig_Nil(t *testing.T) {
	dc := NewDynamicConfig(map[string]interface{}{}, nil)
	err := dc.validateConfig(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestDynamicConfig_ValidateConfig_Valid(t *testing.T) {
	dc := NewDynamicConfig(map[string]interface{}{}, nil)
	err := dc.validateConfig(map[string]string{"key": "value"})
	assert.NoError(t, err)
}
