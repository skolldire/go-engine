package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// closerEntry is a single named resource to be released during shutdown.
type closerEntry struct {
	name  string
	close func(context.Context) error
}

// lifecycle tracks resources created by the builder so they can be released in
// last-in/first-out order during shutdown. LIFO matters because later resources
// may depend on earlier ones. It is safe for concurrent use.
type lifecycle struct {
	mu      sync.Mutex
	entries []closerEntry
	closed  bool
}

// register adds a named closer. A nil function is ignored. Registrations made
// after close are ignored (and reported to the caller as false) so a resource
// created during a failed build is not silently leaked.
func (l *lifecycle) register(name string, fn func(context.Context) error) bool {
	if fn == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	l.entries = append(l.entries, closerEntry{name: name, close: fn})
	return true
}

// close releases every registered resource in LIFO order, aggregating any
// errors with errors.Join. It is idempotent: a second call is a no-op.
func (l *lifecycle) close(ctx context.Context) error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	entries := l.entries
	l.entries = nil
	l.mu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}

	var errs []error
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if err := e.close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("closing %s: %w", e.name, err))
		}
	}
	return errors.Join(errs...)
}

// ensureLifecycle lazily creates the Engine's lifecycle registry.
func (e *Engine) ensureLifecycle() *lifecycle {
	if e.lifecycle == nil {
		e.lifecycle = &lifecycle{}
	}
	return e.lifecycle
}

// registerCloser records a resource to be released on shutdown. Helpers accept
// the concrete Close/Disconnect/Shutdown signatures used across the clients.
func (e *Engine) registerCloser(name string, fn func(context.Context) error) {
	e.ensureLifecycle().register(name, fn)
}

// registerSimpleCloser adapts a Close() error resource (no context) to a closer.
func (e *Engine) registerSimpleCloser(name string, close func() error) {
	if close == nil {
		return
	}
	e.registerCloser(name, func(context.Context) error { return close() })
}

// Close releases every resource registered during construction in LIFO order,
// bounded by ctx. It aggregates errors with errors.Join and is idempotent, so
// it is safe to both wire it into the router's graceful shutdown and defer it.
func (e *Engine) Close(ctx context.Context) error {
	if e.lifecycle == nil {
		return nil
	}
	return e.lifecycle.close(ctx)
}
