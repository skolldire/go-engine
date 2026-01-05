package registry

import (
	"context"
	"fmt"
	"sync"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type ClientFactory func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error)

type Registry struct {
	mu        sync.RWMutex
	factories map[string]ClientFactory
	logger    logger.Service
}

var (
	globalRegistry *Registry
	once           sync.Once
)

// GetRegistry returns the singleton Registry instance.
// The provided logger is used when the registry is first initialized; subsequent calls return the same instance and ignore the logger parameter.
func GetRegistry(log logger.Service) *Registry {
	once.Do(func() {
		globalRegistry = &Registry{
			factories: make(map[string]ClientFactory),
			logger:    log,
		}
	})
	return globalRegistry
}

func (r *Registry) Register(clientType string, factory ClientFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[clientType]; exists {
		return fmt.Errorf("client '%s' already registered", clientType)
	}

	r.factories[clientType] = factory
	if r.logger != nil {
		r.logger.Debug(context.Background(), fmt.Sprintf("client '%s' registered", clientType), nil)
	}
	return nil
}

func (r *Registry) Create(ctx context.Context, clientType string, config interface{}) (interface{}, error) {
	r.mu.RLock()
	factory, exists := r.factories[clientType]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("client '%s' not registered", clientType)
	}

	return factory(ctx, config, r.logger)
}

func (r *Registry) IsRegistered(clientType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[clientType]
	return exists
}

func (r *Registry) ListRegistered() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for clientType := range r.factories {
		types = append(types, clientType)
	}
	return types
}

func (r *Registry) Unregister(clientType string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[clientType]; !exists {
		return fmt.Errorf("client '%s' not registered", clientType)
	}

	delete(r.factories, clientType)
	if r.logger != nil {
		r.logger.Debug(context.Background(), fmt.Sprintf("client '%s' unregistered", clientType), nil)
	}
	return nil
}