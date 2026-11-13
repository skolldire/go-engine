package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
)

var (
	ErrInvalidConfig       = errors.New("invalid configuration")
	ErrSectionMissing      = errors.New("configuration section missing")
	ErrInvalidDocument     = errors.New("invalid configuration document")
	ErrLimitExceeded       = errors.New("configuration limit exceeded")
	ErrUnresolvedReference = errors.New("unresolved configuration reference")
	ErrReferenceNotFound   = errors.New("configuration reference not found")
	ErrReferenceCycle      = errors.New("configuration reference cycle")
	ErrNilContext          = errors.New("nil context")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "config " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	MaxSourceBytes    int64
	MaxDepth          int
	MaxExpansionDepth int
	RejectUnknown     bool
}

func DefaultConfig() Config {
	return Config{MaxSourceBytes: 1 << 20, MaxDepth: 64, MaxExpansionDepth: 8, RejectUnknown: true}
}
func (c Config) Validate() error {
	switch {
	case c.MaxSourceBytes <= 0:
		return &ConfigError{"max_source_bytes", "must be positive"}
	case c.MaxDepth <= 0:
		return &ConfigError{"max_depth", "must be positive"}
	case c.MaxExpansionDepth <= 0:
		return &ConfigError{"max_expansion_depth", "must be positive"}
	case !c.RejectUnknown:
		return &ConfigError{"reject_unknown", "must be true"}
	}
	return nil
}

type Document map[string]any
type Source interface {
	Load(context.Context) (Document, error)
}
type Resolver interface {
	Resolve(context.Context, string) (string, error)
}
type Dependencies struct {
	Sources  []Source
	Resolver Resolver
}
type Loader struct {
	cfg      Config
	sources  []Source
	resolver Resolver
	gate     chan struct{}
}
type Snapshot struct {
	root     *node
	revision string
	maxDepth int
}

func New(cfg Config, deps Dependencies) (*Loader, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	for i, source := range deps.Sources {
		if nilDependency(source) {
			return nil, &ConfigError{fmt.Sprintf("sources[%d]", i), "nil source"}
		}
	}
	if len(deps.Sources) > 0 && int64(len(deps.Sources)) > math.MaxInt64/cfg.MaxSourceBytes {
		return nil, &ConfigError{"sources", "combined byte budget overflows"}
	}
	if deps.Resolver != nil && nilDependency(deps.Resolver) {
		return nil, &ConfigError{"resolver", "nil resolver"}
	}
	return &Loader{cfg: cfg, sources: append([]Source(nil), deps.Sources...), resolver: deps.Resolver, gate: make(chan struct{}, 1)}, nil
}
func (l *Loader) Load(ctx context.Context) (*Snapshot, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if l == nil || l.gate == nil {
		return nil, &ConfigError{"loader", "must be initialized"}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case l.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-l.gate }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := &node{object: map[string]*node{}, origin: "loader"}
	for i, source := range l.sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		origin := fmt.Sprintf("source[%d]", i)
		if named, ok := source.(fmt.Stringer); ok {
			origin = named.String()
		}
		doc, err := source.Load(ctx)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", origin, err)
		}
		budget := measure{limit: l.cfg.MaxSourceBytes, maxDepth: l.cfg.MaxDepth, ctx: ctx}
		incoming, err := budget.copy(map[string]any(doc), "", origin, 1)
		if err != nil {
			return nil, err
		}
		if err := merge(root, incoming, ""); err != nil {
			return nil, err
		}
	}
	resolver := expander{ctx: ctx, resolver: l.resolver, maxDepth: l.cfg.MaxExpansionDepth, cache: map[string]resolved{}, active: map[string]bool{}}
	budget := measure{limit: l.cfg.MaxSourceBytes * int64(len(l.sources)), maxDepth: l.cfg.MaxDepth, ctx: ctx}
	if len(l.sources) == 0 {
		budget.limit = l.cfg.MaxSourceBytes
	}
	if err := resolver.walk(root, "", 1, &budget); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, fmt.Errorf("revision: %w", err)
	}
	return &Snapshot{root: root, revision: hex.EncodeToString(id[:]), maxDepth: l.cfg.MaxDepth}, nil
}
func (s *Snapshot) Revision() string {
	if s == nil {
		return ""
	}
	return s.revision
}
