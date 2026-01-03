package dynamic

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type FileWatcher struct {
	paths    []string
	watcher  *fsnotify.Watcher
	logger   logger.Service
	debounce time.Duration
	stopCh   chan struct{}
}

func NewFileWatcher(paths []string, log logger.Service) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	fw := &FileWatcher{
		paths:    paths,
		watcher:  watcher,
		logger:   log,
		debounce: 500 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}

	for _, path := range paths {
		if err := watcher.Add(path); err != nil {
			watcher.Close()
			return nil, fmt.Errorf("failed to add path to watcher: %w", err)
		}
	}

	return fw, nil
}

func (fw *FileWatcher) Watch(ctx context.Context, onChange func() error) error {
	debounceTimer := time.NewTimer(fw.debounce)
	debounceTimer.Stop()
	var lastEvent time.Time

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-fw.stopCh:
			return nil
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return nil
			}

			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				now := time.Now()
				if now.Sub(lastEvent) < fw.debounce {
					debounceTimer.Reset(fw.debounce)
					continue
				}

				if fw.isConfigFile(event.Name) {
					fw.logger.Debug(ctx, "configuration file modified", map[string]interface{}{
						"file": event.Name,
						"op":   event.Op.String(),
					})

					time.Sleep(100 * time.Millisecond)

					if err := onChange(); err != nil {
						fw.logger.Error(ctx, err, map[string]interface{}{
							"event": "config_reload_error",
							"file":  event.Name,
						})
					}

					lastEvent = time.Now()
				}
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return nil
			}
			fw.logger.Error(ctx, err, map[string]interface{}{
				"event": "watcher_error",
			})
		case <-debounceTimer.C:
		}
	}
}

func (fw *FileWatcher) Stop() error {
	close(fw.stopCh)
	return fw.watcher.Close()
}

func (fw *FileWatcher) isConfigFile(filename string) bool {
	ext := filepath.Ext(filename)
	return ext == ".yaml" || ext == ".yml" || ext == ".json" || ext == ".toml"
}

