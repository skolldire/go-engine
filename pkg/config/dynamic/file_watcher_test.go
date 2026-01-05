package dynamic

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockLogger is now defined in mocks_test.go

func TestNewFileWatcher(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_watcher")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	tempFile := filepath.Join(tempDir, "test.yaml")
	f, err := os.Create(tempFile)
	assert.NoError(t, err)
	f.Close()
	
	watcher, err := NewFileWatcher([]string{tempFile}, nil)
	assert.NoError(t, err)
	assert.NotNil(t, watcher)
	defer watcher.Stop()
}

func TestNewFileWatcher_InvalidPath(t *testing.T) {
	watcher, err := NewFileWatcher([]string{"/nonexistent/path/file.yaml"}, nil)
	assert.Error(t, err)
	assert.Nil(t, watcher)
}

func TestFileWatcher_Stop(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_watcher")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	tempFile := filepath.Join(tempDir, "test.yaml")
	f, err := os.Create(tempFile)
	assert.NoError(t, err)
	f.Close()
	
	watcher, err := NewFileWatcher([]string{tempFile}, nil)
	assert.NoError(t, err)
	
	err = watcher.Stop()
	assert.NoError(t, err)
}

func TestFileWatcher_IsConfigFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{"yaml", "config.yaml", true},
		{"yml", "config.yml", true},
		{"json", "config.json", true},
		{"toml", "config.toml", true},
		{"txt", "config.txt", false},
		{"no ext", "config", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "test_watcher")
			assert.NoError(t, err)
			defer os.RemoveAll(tempDir)
			
			tempFile := filepath.Join(tempDir, tt.filename)
			f, err := os.Create(tempFile)
			assert.NoError(t, err)
			f.Close()
			
			watcher, err := NewFileWatcher([]string{tempFile}, nil)
			if err != nil {
				return // Skip if watcher creation fails
			}
			defer watcher.Stop()
			
			result := watcher.isConfigFile(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFileWatcher_Watch_ContextCancelled(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_watcher")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	tempFile := filepath.Join(tempDir, "test.yaml")
	f, err := os.Create(tempFile)
	assert.NoError(t, err)
	f.Close()
	
	watcher, err := NewFileWatcher([]string{tempFile}, nil)
	assert.NoError(t, err)
	defer watcher.Stop()
	
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	err = watcher.Watch(ctx, func() error {
		return nil
	})
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

