package dynamic

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type FeatureFlags struct {
	flags  atomic.Value
	mu     sync.RWMutex
	logger logger.Service
}

func NewFeatureFlags(initialFlags map[string]any, log logger.Service) *FeatureFlags {
	ff := &FeatureFlags{
		logger: log,
	}
	if initialFlags == nil {
		initialFlags = make(map[string]any)
	}
	ff.flags.Store(initialFlags)
	return ff
}

func (ff *FeatureFlags) Get(key string) (any, bool) {
	flags := ff.flags.Load().(map[string]any)
	value, exists := flags[key]
	return value, exists
}

func (ff *FeatureFlags) GetBool(key string) bool {
	value, exists := ff.Get(key)
	if !exists {
		return false
	}

	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1" || v == "yes"
	default:
		return false
	}
}

func (ff *FeatureFlags) GetString(key string) string {
	value, exists := ff.Get(key)
	if !exists {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

func (ff *FeatureFlags) GetInt(key string) int {
	value, exists := ff.Get(key)
	if !exists {
		return 0
	}

	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

func (ff *FeatureFlags) Set(key string, value any) {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	flags := ff.flags.Load().(map[string]any)
	newFlags := make(map[string]any)
	for k, v := range flags {
		newFlags[k] = v
	}
	newFlags[key] = value
	ff.flags.Store(newFlags)

	ff.logger.Debug(context.Background(), "feature flag updated", map[string]any{
		"key":   key,
		"value": value,
	})
}

func (ff *FeatureFlags) SetAll(flags map[string]any) {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	if flags == nil {
		flags = make(map[string]any)
	}
	ff.flags.Store(flags)

	ff.logger.Info(context.Background(), "feature flags updated", map[string]any{
		"count": len(flags),
	})
}

func (ff *FeatureFlags) GetAll() map[string]any {
	flags := ff.flags.Load().(map[string]any)
	result := make(map[string]any)
	for k, v := range flags {
		result[k] = v
	}
	return result
}

func (ff *FeatureFlags) IsEnabled(key string) bool {
	return ff.GetBool(key)
}
