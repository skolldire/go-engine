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

// NewFeatureFlags creates a FeatureFlags manager initialized with the provided flag set and logger.
// If initialFlags is nil, an empty flag set is used. The returned *FeatureFlags is ready for concurrent use.
func NewFeatureFlags(initialFlags map[string]interface{}, log logger.Service) *FeatureFlags {
	ff := &FeatureFlags{
		logger: log,
	}
	if initialFlags == nil {
		initialFlags = make(map[string]interface{})
	}
	ff.flags.Store(initialFlags)
	return ff
}

func (ff *FeatureFlags) Get(key string) (interface{}, bool) {
	flags := ff.flags.Load().(map[string]interface{})
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
		fmt.Sscanf(v, "%d", &i)
		return i
	default:
		return 0
	}
}

func (ff *FeatureFlags) Set(key string, value interface{}) {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	flags := ff.flags.Load().(map[string]interface{})
	newFlags := make(map[string]interface{})
	for k, v := range flags {
		newFlags[k] = v
	}
	newFlags[key] = value
	ff.flags.Store(newFlags)

	ff.logger.Debug(context.Background(), "feature flag updated", map[string]interface{}{
		"key":   key,
		"value": value,
	})
}

func (ff *FeatureFlags) SetAll(flags map[string]interface{}) {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	if flags == nil {
		flags = make(map[string]interface{})
	}
	ff.flags.Store(flags)

	ff.logger.Info(context.Background(), "feature flags updated", map[string]interface{}{
		"count": len(flags),
	})
}

func (ff *FeatureFlags) GetAll() map[string]interface{} {
	flags := ff.flags.Load().(map[string]interface{})
	result := make(map[string]interface{})
	for k, v := range flags {
		result[k] = v
	}
	return result
}

func (ff *FeatureFlags) IsEnabled(key string) bool {
	return ff.GetBool(key)
}
