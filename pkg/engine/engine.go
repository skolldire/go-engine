package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// Engine is the assembled application: core services plus whatever components
// its providers contributed. It holds no adapter-specific field, which is what
// allows an adapter to be added without touching this file.
type Engine struct {
	ctx context.Context
	log logger.Service

	// components maps a provider Name to the value its Init returned.
	componentsMu sync.RWMutex
	components   map[string]any

	// resources memoizes shared objects built on demand by providers.
	resourcesMu sync.Mutex
	resources   map[string]*resourceEntry

	// providers are the claimed Provider values, released on Close so the same
	// instance can be reused once this engine is done with it.
	providers []Provider

	// closeOnce guards the provider release: lifecycle.close is already
	// idempotent, but the release loop reads and clears e.providers, so two
	// concurrent Close calls could double-release or observe a torn slice.
	closeOnce sync.Once

	core      *coreServices
	lifecycle *lifecycle
	telemetry *telemetrySwitch
}

// Telemetry returns the engine's telemetry implementation. It is never nil: it
// is a no-op unless a telemetry provider was registered.
func (e *Engine) Telemetry() Telemetry {
	if e.telemetry == nil {
		return noopTelemetry{}
	}
	return e.telemetry
}

type resourceEntry struct {
	once  sync.Once
	value any
	err   error
}

// Context returns the context the engine was built with.
func (e *Engine) Context() context.Context { return e.ctx }

// Logger returns the engine logger. Never nil.
func (e *Engine) Logger() logger.Service { return e.log }

// Close releases every component in LIFO order, bounded by ctx. Errors are
// aggregated with errors.Join. It is idempotent.
func (e *Engine) Close(ctx context.Context) error {
	if e.lifecycle == nil {
		return nil
	}
	err := e.lifecycle.close(ctx)

	// Release the single-use claim exactly once: after Close the engine owns
	// nothing, so the provider instances may legitimately be handed to a new
	// engine. Concurrent Close calls must not double-release.
	e.closeOnce.Do(func() {
		for _, p := range e.providers {
			releaseProvider(p)
		}
		e.providers = nil
	})

	return err
}

// RegisterCloser records an extra resource to release at shutdown. Components
// registered by providers are handled automatically; this is for resources the
// application itself owns.
func (e *Engine) RegisterCloser(name string, fn func(context.Context) error) {
	e.lifecycle.register(name, fn)
}

// Component returns the raw value a provider contributed under name. Prefer the
// typed helper Get, or the From helper each provider package exposes.
func (e *Engine) Component(name string) (any, bool) {
	e.componentsMu.RLock()
	defer e.componentsMu.RUnlock()
	v, ok := e.components[name]
	return v, ok
}

// ComponentNames lists every registered component, sorted. Useful in tests and
// in diagnostics endpoints.
func (e *Engine) ComponentNames() []string {
	e.componentsMu.RLock()
	defer e.componentsMu.RUnlock()

	names := make([]string, 0, len(e.components))
	for name := range e.components {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (e *Engine) setComponent(name string, value any) {
	e.componentsMu.Lock()
	defer e.componentsMu.Unlock()
	e.components[name] = value
}

// resource memoizes build under key for the engine's lifetime. Concurrent
// callers share one execution and one result.
func (e *Engine) resource(key string, build func() (any, error)) (any, error) {
	e.resourcesMu.Lock()
	entry, ok := e.resources[key]
	if !ok {
		entry = &resourceEntry{}
		e.resources[key] = entry
	}
	e.resourcesMu.Unlock()

	entry.once.Do(func() {
		entry.value, entry.err = build()
	})
	return entry.value, entry.err
}

// Get retrieves a component by name with its concrete type.
//
// It is the type-safe replacement for the old per-adapter getters: instead of
// Engine growing a GetSQSClientByName method (and an import of the SQS package)
// for every adapter, the type travels with the caller.
func Get[T any](e *Engine, name string) (T, error) {
	var zero T

	raw, ok := e.Component(name)
	if !ok {
		return zero, fmt.Errorf("component %q not found; registered: %v", name, e.ComponentNames())
	}

	typed, ok := raw.(T)
	if !ok {
		return zero, fmt.Errorf("component %q is %T, not %T", name, raw, zero)
	}
	return typed, nil
}

// MustGet is Get for application startup, where a missing or mistyped component
// is a programming error rather than a runtime condition.
func MustGet[T any](e *Engine, name string) T {
	typed, err := Get[T](e, name)
	if err != nil {
		panic(err)
	}
	return typed
}

// sortStrings avoids importing sort in more than one file of the core.
func sortStrings(s []string) { sort.Strings(s) }
