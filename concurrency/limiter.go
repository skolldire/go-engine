package concurrency

import (
	"context"
	"sync"
)

type LimiterConfig struct {
	Capacity int64 `json:"capacity" yaml:"capacity" mapstructure:"capacity"`
}
type limiterWaiter struct {
	weight  int64
	ready   chan struct{}
	granted bool
}

type Limiter struct {
	mu             sync.Mutex
	capacity, used int64
	initialized    bool
	waiters        []*limiterWaiter
}

func NewLimiter(cfg LimiterConfig) (*Limiter, error) {
	if cfg.Capacity <= 0 {
		return nil, invalid("capacity", "must be positive")
	}
	return &Limiter{capacity: cfg.Capacity, initialized: true}, nil
}

func (l *Limiter) Acquire(ctx context.Context, weight int64) (func(), error) {
	if l == nil || !l.initialized || ctx == nil {
		return nil, invalid("limiter", "requires initialized limiter and non-nil context")
	}
	if weight <= 0 {
		return nil, invalid("weight", "must be positive")
	}
	if weight > l.capacity {
		return nil, ErrJobTooLarge
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w := &limiterWaiter{weight: weight, ready: make(chan struct{})}
	l.mu.Lock()
	l.waiters = append(l.waiters, w)
	l.grantLocked()
	l.mu.Unlock()
	select {
	case <-w.ready:
	case <-ctx.Done():
	}
	l.mu.Lock()
	if err := ctx.Err(); err != nil {
		if w.granted {
			l.used -= weight
		} else {
			for i, item := range l.waiters {
				if item == w {
					copy(l.waiters[i:], l.waiters[i+1:])
					l.waiters[len(l.waiters)-1] = nil
					l.waiters = l.waiters[:len(l.waiters)-1]
					break
				}
			}
		}
		l.grantLocked()
		l.mu.Unlock()
		return nil, err
	}
	l.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { l.mu.Lock(); l.used -= weight; l.grantLocked(); l.mu.Unlock() }) }, nil
}
func (l *Limiter) grantLocked() {
	for len(l.waiters) > 0 {
		w := l.waiters[0]
		if w.weight > l.capacity-l.used {
			return
		}
		l.waiters[0] = nil
		l.waiters = l.waiters[1:]
		l.used += w.weight
		w.granted = true
		close(w.ready)
	}
	if len(l.waiters) == 0 {
		l.waiters = nil
	}
}
