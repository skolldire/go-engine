package memcached

import (
	"context"
	"fmt"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

func NewClient(cfg Config, log logger.Service) (Service, error) {
	if len(cfg.Servers) == 0 {
		return nil, ErrConnection
	}

	mc := memcache.New(cfg.Servers...)

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	mc.Timeout = timeout

	maxIdleConns := cfg.MaxIdleConns
	if maxIdleConns == 0 {
		maxIdleConns = DefaultMaxIdleConns
	}
	mc.MaxIdleConns = maxIdleConns

	baseConfig := client.BaseConfig{
		EnableLogging:  cfg.EnableLogging,
		WithResilience: cfg.WithResilience,
		Resilience:     cfg.Resilience,
		Timeout:        timeout,
	}

	c := &MemcachedClient{
		BaseClient: client.NewBaseClientWithName(baseConfig, log, "Memcached"),
		client:     mc,
		prefix:     cfg.Prefix,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	if c.IsLoggingEnabled() {
		log.Debug(ctx, "Memcached connection established successfully",
			map[string]interface{}{
				"servers": cfg.Servers,
			})
	}

	return c, nil
}

func (c *MemcachedClient) Ping(ctx context.Context) error {
	_, err := c.Execute(ctx, "Ping", func() (interface{}, error) {
		return nil, c.client.Ping()
	})
	return err
}

func (c *MemcachedClient) Get(ctx context.Context, key string) ([]byte, error) {
	fullKey := c.KeyName(key)

	result, err := c.Execute(ctx, "Get", func() (interface{}, error) {
		item, err := c.client.Get(fullKey)
		if err != nil {
			if err == memcache.ErrCacheMiss {
				return nil, ErrKeyNotFound
			}
			return nil, err
		}
		return item.Value, nil
	})

	if err != nil {
		return nil, err
	}

	value, err := client.SafeTypeAssert[[]byte](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected response type from memcached Get: %w", err)
	}
	return value, nil
}

func (c *MemcachedClient) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	fullKey := c.KeyName(key)

	if expiration == 0 {
		expiration = DefaultExpiration
	}

	_, err := c.Execute(ctx, "Set", func() (interface{}, error) {
		return nil, c.client.Set(&memcache.Item{
			Key:        fullKey,
			Value:      value,
			Expiration: int32(expiration.Seconds()),
		})
	})

	return err
}

func (c *MemcachedClient) Delete(ctx context.Context, key string) error {
	fullKey := c.KeyName(key)

	_, err := c.Execute(ctx, "Delete", func() (interface{}, error) {
		return nil, c.client.Delete(fullKey)
	})

	return err
}

func (c *MemcachedClient) Add(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	fullKey := c.KeyName(key)

	if expiration == 0 {
		expiration = DefaultExpiration
	}

	_, err := c.Execute(ctx, "Add", func() (interface{}, error) {
		return nil, c.client.Add(&memcache.Item{
			Key:        fullKey,
			Value:      value,
			Expiration: int32(expiration.Seconds()),
		})
	})

	return err
}

func (c *MemcachedClient) Replace(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	fullKey := c.KeyName(key)

	if expiration == 0 {
		expiration = DefaultExpiration
	}

	_, err := c.Execute(ctx, "Replace", func() (interface{}, error) {
		return nil, c.client.Replace(&memcache.Item{
			Key:        fullKey,
			Value:      value,
			Expiration: int32(expiration.Seconds()),
		})
	})

	return err
}

func (c *MemcachedClient) Increment(ctx context.Context, key string, delta uint64) (uint64, error) {
	fullKey := c.KeyName(key)

	result, err := c.Execute(ctx, "Increment", func() (interface{}, error) {
		return c.client.Increment(fullKey, delta)
	})

	if err != nil {
		return 0, err
	}

	value, err := client.SafeTypeAssert[uint64](result)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func (c *MemcachedClient) Decrement(ctx context.Context, key string, delta uint64) (uint64, error) {
	fullKey := c.KeyName(key)

	result, err := c.Execute(ctx, "Decrement", func() (interface{}, error) {
		return c.client.Decrement(fullKey, delta)
	})

	if err != nil {
		return 0, err
	}

	value, err := client.SafeTypeAssert[uint64](result)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func (c *MemcachedClient) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = c.KeyName(key)
	}

	result, err := c.Execute(ctx, "GetMulti", func() (interface{}, error) {
		items, err := c.client.GetMulti(fullKeys)
		if err != nil {
			return nil, err
		}

		data := make(map[string][]byte)
		for k, item := range items {
			originalKey := c.removePrefix(k)
			data[originalKey] = item.Value
		}
		return data, nil
	})

	if err != nil {
		return nil, err
	}

	data, err := client.SafeTypeAssert[map[string][]byte](result)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (c *MemcachedClient) FlushAll(ctx context.Context) error {
	_, err := c.Execute(ctx, "FlushAll", func() (interface{}, error) {
		return nil, c.client.FlushAll()
	})

	return err
}

func (c *MemcachedClient) KeyName(key string) string {
	if c.prefix == "" {
		return key
	}
	return fmt.Sprintf("%s:%s", c.prefix, key)
}

func (c *MemcachedClient) removePrefix(key string) string {
	if c.prefix == "" {
		return key
	}
	prefix := c.prefix + ":"
	if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
		return key[len(prefix):]
	}
	return key
}

func (c *MemcachedClient) EnableLogging(enable bool) {
	c.SetLogging(enable)
}
