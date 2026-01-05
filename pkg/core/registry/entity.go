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

func GetRegistry() *Registry {
	once.Do(func() {
		globalRegistry = &Registry{
			factories: make(map[string]ClientFactory),
		}
	})
	return globalRegistry
}

func (r *Registry) SetLogger(log logger.Service) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger = log
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
	logger := r.logger // Copy logger while holding lock
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("client '%s' not registered", clientType)
	}

	return factory(ctx, config, logger)
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
