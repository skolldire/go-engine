package dynamic

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type DynamicConfig struct {
	config      atomic.Value
	mu          sync.RWMutex
	logger      logger.Service
	reloadFunc  func() (interface{}, error)
	watchers    []ConfigWatcher
	reloadHooks []ReloadHook
	lastReload  atomic.Value
}

type ConfigWatcher interface {
	Watch(ctx context.Context, onChange func() error) error
	Stop() error
}

type ReloadHook func(oldConfig, newConfig interface{}) error

func NewDynamicConfig(initialConfig interface{}, log logger.Service) *DynamicConfig {
	dc := &DynamicConfig{
		logger:      log,
		watchers:    make([]ConfigWatcher, 0),
		reloadHooks: make([]ReloadHook, 0),
	}

	dc.config.Store(initialConfig)
	dc.lastReload.Store(time.Now())

	return dc
}

func (dc *DynamicConfig) Get() interface{} {
	return dc.config.Load()
}

func (dc *DynamicConfig) SetReloadFunc(fn func() (interface{}, error)) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.reloadFunc = fn
}

func (dc *DynamicConfig) Reload() error {
	if dc.reloadFunc == nil {
		return nil
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	oldConfig := dc.Get()
	newConfig, err := dc.reloadFunc()
	if err != nil {
		dc.logger.Error(context.Background(), err, map[string]interface{}{
			"event": "config_reload_failed",
		})
		return err
	}

	if err := dc.validateConfig(newConfig); err != nil {
		dc.logger.Error(context.Background(), err, map[string]interface{}{
			"event": "config_validation_failed",
		})
		return err
	}

	for _, hook := range dc.reloadHooks {
		if err := hook(oldConfig, newConfig); err != nil {
			dc.logger.Warn(context.Background(), "reload hook failed: "+err.Error(), map[string]interface{}{
				"event": "reload_hook_failed",
			})
		}
	}

	dc.config.Store(newConfig)
	dc.lastReload.Store(time.Now())

	dc.logger.Info(context.Background(), "configuration reloaded successfully", map[string]interface{}{
		"event": "config_reloaded",
	})

	return nil
}

func (dc *DynamicConfig) AddWatcher(watcher ConfigWatcher) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.watchers = append(dc.watchers, watcher)
}

func (dc *DynamicConfig) AddReloadHook(hook ReloadHook) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.reloadHooks = append(dc.reloadHooks, hook)
}

func (dc *DynamicConfig) StartWatching(ctx context.Context) error {
	dc.mu.RLock()
	watchers := make([]ConfigWatcher, len(dc.watchers))
	copy(watchers, dc.watchers)
	dc.mu.RUnlock()

	for _, watcher := range watchers {
		w := watcher
		go func() {
			if err := w.Watch(ctx, func() error {
				return dc.Reload()
			}); err != nil {
				dc.logger.Error(ctx, err, map[string]interface{}{
					"event": "watcher_error",
				})
			}
		}()
	}

	return nil
}

func (dc *DynamicConfig) Stop() error {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	var errs []error
	for _, watcher := range dc.watchers {
		if err := watcher.Stop(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (dc *DynamicConfig) GetLastReload() time.Time {
	lastReload := dc.lastReload.Load()
	if lastReload == nil {
		return time.Time{}
	}
	return lastReload.(time.Time)
}

func (dc *DynamicConfig) validateConfig(config interface{}) error {
	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}
	return nil
}
