package memcached

import (
	"context"
	"errors"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout      = 5 * time.Second
	DefaultExpiration   = 24 * time.Hour
	DefaultMaxIdleConns = 2
)

var (
	ErrKeyNotFound  = errors.New("key not found")
	ErrInvalidValue = errors.New("invalid value")
	ErrConnection   = errors.New("memcached connection error")
)

type Config struct {
	Servers        []string          `mapstructure:"servers" json:"servers"`
	Timeout        time.Duration     `mapstructure:"timeout" json:"timeout"`
	MaxIdleConns   int               `mapstructure:"max_idle_conns" json:"max_idle_conns"`
	Prefix         string            `mapstructure:"prefix" json:"prefix"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
}

type Service interface {
	// Get retrieves a value by key. Returns ErrKeyNotFound if key doesn't exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value with optional expiration.
	// If expiration is 0, uses DefaultExpiration.
	Set(ctx context.Context, key string, value []byte, expiration time.Duration) error

	// Delete removes a key from the cache.
	Delete(ctx context.Context, key string) error

	// Add stores a value only if the key doesn't already exist.
	Add(ctx context.Context, key string, value []byte, expiration time.Duration) error

	// Replace updates a value only if the key already exists.
	Replace(ctx context.Context, key string, value []byte, expiration time.Duration) error

	// Increment increments a numeric value by delta.
	// The key must exist and contain a numeric value.
	Increment(ctx context.Context, key string, delta uint64) (uint64, error)

	// Decrement decrements a numeric value by delta.
	// The key must exist and contain a numeric value.
	Decrement(ctx context.Context, key string, delta uint64) (uint64, error)

	// GetMulti retrieves multiple keys at once.
	// Returns a map of found keys (missing keys are not included).
	GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)

	// FlushAll removes all keys from all servers.
	FlushAll(ctx context.Context) error

	// KeyName returns the full key name with prefix applied.
	KeyName(key string) string

	// EnableLogging enables or disables logging for this client.
	EnableLogging(enable bool)
}

type MemcachedClient struct {
	*client.BaseClient
	client *memcache.Client
	prefix string
}
