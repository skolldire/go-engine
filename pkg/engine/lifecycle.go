package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type closerEntry struct {
	name  string
	close func(context.Context) error
}

// lifecycle releases resources in last-in/first-out order. LIFO matters because
// a component built later may depend on one built earlier. Safe for concurrent
// use; registrations after close are rejected so a resource created during a
// failed build is never silently leaked.
type lifecycle struct {
	mu      sync.Mutex
	entries []closerEntry
	closed  bool
}

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
