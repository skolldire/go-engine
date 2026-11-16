package jwt

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

const unknownKeyRefreshInterval = 30 * time.Second

type flight struct {
	done    chan struct{}
	err     error
	unknown bool
}
type Verifier struct {
	cfg        Config
	client     *http.Client
	mu         sync.Mutex
	keys       map[string]verificationKey
	fetched    time.Time
	active     *flight
	unknownAt  time.Time
	unknownErr error
}

func NewVerifier(cfg Config, deps Dependencies) (*Verifier, error) {
	if e := cfg.Validate(); e != nil {
		return nil, e
	}
	if deps.HTTPClient == nil {
		return nil, &ConfigError{"HTTPClient", "is required"}
	}
	cfg.Algorithms = append([]string(nil), cfg.Algorithms...)
	return &Verifier{cfg: cfg, client: deps.HTTPClient}, nil
}
func canceled(ctx context.Context) error { return errors.Join(ctx.Err(), context.Cause(ctx)) }
func wait(ctx context.Context, f *flight) error {
	select {
	case <-ctx.Done():
		return canceled(ctx)
	case <-f.done:
		if ctx.Err() != nil {
			return canceled(ctx)
		}
		return f.err
	}
}

// start is called under mu; HTTP and dependency callbacks run in the worker,
// never while the state lock is held.
func (v *Verifier) start(ctx context.Context, unknown bool) *flight {
	if v.active != nil {
		return v.active
	}
	f := &flight{done: make(chan struct{}), unknown: unknown}
	v.active = f
	if unknown {
		v.unknownAt = time.Now()
		v.unknownErr = nil
	}
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), v.cfg.RefreshTimeout)
	go func() {
		defer cancel()
		keys, e := v.fetch(fetchCtx)
		if e == nil && fetchCtx.Err() != nil {
			e = canceled(fetchCtx)
		}
		if e != nil {
			e = unavailable(e)
		}
		v.mu.Lock()
		if e == nil {
			v.keys = keys
			v.fetched = time.Now()
		}
		f.err = e
		if f.unknown {
			v.unknownErr = e
		}
		v.active = nil
		close(f.done)
		v.mu.Unlock()
	}()
	return f
}
func (v *Verifier) Refresh(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	if ctx.Err() != nil {
		return canceled(ctx)
	}
	v.mu.Lock()
	f := v.start(ctx, false)
	v.mu.Unlock()
	return wait(ctx, f)
}
func (v *Verifier) key(ctx context.Context, kid string) (verificationKey, error) {
	v.mu.Lock()
	key, exists := v.keys[kid]
	now := time.Now()
	fresh := !v.fetched.IsZero() && now.Before(v.fetched.Add(v.cfg.CacheTTL))
	if exists && fresh {
		v.mu.Unlock()
		return key, nil
	}
	if fresh && (v.active == nil || !v.active.unknown) && !v.unknownAt.IsZero() && time.Since(v.unknownAt) < unknownKeyRefreshInterval {
		e := v.unknownErr
		v.mu.Unlock()
		if e != nil {
			return verificationKey{}, e
		}
		return verificationKey{}, ErrUnknownKey
	}
	// Mark only an unknown-kid-triggered fetch, not the initial/TTL load.
	f := v.start(ctx, fresh && !exists)
	v.mu.Unlock()
	e := wait(ctx, f)
	if e != nil {
		if ctx.Err() != nil {
			return verificationKey{}, e
		}
		v.mu.Lock()
		key, exists = v.keys[kid]
		now = time.Now()
		stale := exists && !v.fetched.IsZero() && !now.Before(v.fetched.Add(v.cfg.CacheTTL)) && now.Before(v.fetched.Add(v.cfg.CacheTTL).Add(v.cfg.StaleGrace))
		v.mu.Unlock()
		if stale {
			return key, nil
		}
		return verificationKey{}, e
	}
	v.mu.Lock()
	key, exists = v.keys[kid]
	v.mu.Unlock()
	if !exists {
		return verificationKey{}, ErrUnknownKey
	}
	return key, nil
}
