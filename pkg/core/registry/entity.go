package registry

import (
	"context"
	"fmt"
	"sync"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// ClientFactory is a constructor function that the Registry invokes when Create is called.
// It receives the calling context, an opaque config value, and the registry's logger.
// The returned value is the fully-initialised client instance.
type ClientFactory func(ctx context.Context, config interface{}, log logger.Service) (interface{}, error)

// Registry is a thread-safe factory registry that maps client-type names to their
// ClientFactory functions. It is used internally by go-engine to defer client
// construction until the first Create call.
//
// Thread safety: all methods acquire the internal RWMutex and are safe to call
// from multiple goroutines concurrently.
//
// Behaviour contract:
//   - Register: returns an error if the clientType is already registered; does NOT overwrite.
//   - Create:   returns an error if the clientType has not been registered.
//   - Unregister: returns an error if the clientType is not currently registered.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]ClientFactory
	logger    logger.Service
}

var (
	globalRegistry *Registry
	once           sync.Once
)

// GetRegistry returns the process-wide singleton Registry.
// The singleton is initialised on the first call via sync.Once and is safe for
// concurrent use from that point on. Subsequent calls always return the same instance.
func GetRegistry() *Registry {
	once.Do(func() {
		globalRegistry = &Registry{
			factories: make(map[string]ClientFactory),
		}
	})
	return globalRegistry
}

// SetLogger attaches a logger to the registry. Debug entries are emitted on
// Register and Unregister when a logger is present.
func (r *Registry) SetLogger(log logger.Service) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger = log
}

// Register associates clientType with factory.
// Returns an error if clientType is already registered; it does not overwrite
// an existing factory. To replace a factory, call Unregister first.
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

// Create invokes the factory registered under clientType, passing ctx and config.
// Returns an error if clientType has not been registered, or if the factory itself
// returns an error.
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

// IsRegistered reports whether a factory for clientType has been registered.
func (r *Registry) IsRegistered(clientType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[clientType]
	return exists
}

// ListRegistered returns the names of all currently registered client types.
// The order of the returned slice is non-deterministic (map iteration order).
func (r *Registry) ListRegistered() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for clientType := range r.factories {
		types = append(types, clientType)
	}
	return types
}

// Unregister removes the factory for clientType.
// Returns an error if clientType is not currently registered.
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
