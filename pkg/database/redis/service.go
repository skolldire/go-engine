package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
)

func NewClient(cfg Config, log logger.Service) (*RedisClient, error) {
	options := &redis.Options{
		Addr:        fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		DB:          cfg.DB,
		DialTimeout: time.Duration(cfg.Timeout) * time.Second,
	}
	if cfg.Password != "" {
		options.Password = cfg.Password
	}

	client := redis.NewClient(options)

	rc := &RedisClient{
		client:    client,
		logger:    log,
		logging:   cfg.EnableLogging,
		keyPrefix: cfg.Prefix,
	}

	if cfg.RetryConfig != nil {
		rc.retryer = retry_backoff.NewRetryer(retry_backoff.Dependencies{
			RetryConfig: cfg.RetryConfig,
			Logger:      log,
		})
	}

	if cfg.CircuitBreakerCfg != nil {
		rc.circuitBreaker = circuit_breaker.NewCircuitBreaker(circuit_breaker.Dependencies{
			Config: cfg.CircuitBreakerCfg,
			Log:    log,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	if err := rc.Ping(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	if rc.logging {
		log.Debug(ctx, "Conexión a Redis establecida correctamente",
			map[string]interface{}{"host": cfg.Host, "port": cfg.Port})
	}

	return rc, nil
}

// KeyName prefija la clave
func (rc *RedisClient) KeyName(key string) string {
	if rc.keyPrefix == "" {
		return key
	}
	return fmt.Sprintf("%s:%s", rc.keyPrefix, key)
}

// ensureContextWithTimeout asegura que el contexto tenga un timeout
func (rc *RedisClient) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return context.WithTimeout(ctx, DefaultTimeout)
	}
	return context.WithCancel(ctx)
}

func (rc *RedisClient) execute(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := rc.ensureContextWithTimeout(ctx)
	defer cancel()

	if rc.circuitBreaker != nil && rc.retryer != nil {
		return rc.executeWithCircuitBreakerAndRetry(ctx, operationName, operation)
	}

	if rc.circuitBreaker != nil {
		return rc.executeWithCircuitBreaker(ctx, operationName, operation)
	}

	if rc.retryer != nil {
		return rc.executeWithRetry(ctx, operationName, operation)
	}

	return rc.executeWithLogging(ctx, operationName, operation)
}

func (rc *RedisClient) executeWithLogging(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	logFields := map[string]interface{}{"operation": operationName}

	if rc.logging {
		rc.logger.Debug(ctx, fmt.Sprintf("Iniciando operación Redis: %s", operationName), logFields)
	}

	result, err := operation()

	if err != nil && rc.logging {
		rc.logger.Error(ctx, err, logFields)
	} else if rc.logging {
		rc.logger.Debug(ctx, fmt.Sprintf("Operación Redis completada: %s", operationName), logFields)
	}

	return result, err
}

func (rc *RedisClient) executeWithCircuitBreakerAndRetry(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	return rc.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return rc.executeWithRetryInner(ctx, operationName, operation)
	})
}

func (rc *RedisClient) executeWithCircuitBreaker(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	return rc.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return rc.executeWithLogging(ctx, operationName, operation)
	})
}

func (rc *RedisClient) executeWithRetry(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	return rc.executeWithRetryInner(ctx, operationName, operation)
}

func (rc *RedisClient) executeWithRetryInner(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	var result interface{}

	err := rc.retryer.Do(ctx, func() error {
		res, opErr := rc.executeWithLogging(ctx, operationName, operation)
		if opErr == nil {
			result = res
		}
		return opErr
	})

	return result, err
}

func (rc *RedisClient) Ping(ctx context.Context) error {
	_, err := rc.execute(ctx, "Ping", func() (interface{}, error) {
		return rc.client.Ping(ctx).Result()
	})
	return err
}

func (rc *RedisClient) Get(ctx context.Context, key string) (string, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "Get", func() (interface{}, error) {
		return rc.client.Get(ctx, prefixedKey).Result()
	})

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrKeyNotFound
		}
		return "", err
	}

	value, ok := result.(string)
	if !ok {
		return "", ErrInvalidValue
	}

	return value, nil
}

func (rc *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	prefixedKey := rc.KeyName(key)

	_, err := rc.execute(ctx, "Set", func() (interface{}, error) {
		return rc.client.Set(ctx, prefixedKey, value, expiration).Result()
	})

	return err
}

func (rc *RedisClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "SetNX", func() (interface{}, error) {
		return rc.client.SetNX(ctx, prefixedKey, value, expiration).Result()
	})

	if err != nil {
		return false, err
	}

	success, ok := result.(bool)
	if !ok {
		return false, ErrInvalidValue
	}

	return success, nil
}

func (rc *RedisClient) Del(ctx context.Context, keys ...string) (int64, error) {
	prefixedKeys := make([]string, len(keys))
	for i, key := range keys {
		prefixedKeys[i] = rc.KeyName(key)
	}

	result, err := rc.execute(ctx, "Del", func() (interface{}, error) {
		return rc.client.Del(ctx, prefixedKeys...).Result()
	})

	if err != nil {
		return 0, err
	}

	count, ok := result.(int64)
	if !ok {
		return 0, ErrInvalidValue
	}

	return count, nil
}

func (rc *RedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	prefixedKeys := make([]string, len(keys))
	for i, key := range keys {
		prefixedKeys[i] = rc.KeyName(key)
	}

	result, err := rc.execute(ctx, "Exists", func() (interface{}, error) {
		return rc.client.Exists(ctx, prefixedKeys...).Result()
	})

	if err != nil {
		return 0, err
	}

	count, ok := result.(int64)
	if !ok {
		return 0, ErrInvalidValue
	}

	return count, nil
}

func (rc *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "Expire", func() (interface{}, error) {
		return rc.client.Expire(ctx, prefixedKey, expiration).Result()
	})

	if err != nil {
		return false, err
	}

	success, ok := result.(bool)
	if !ok {
		return false, ErrInvalidValue
	}

	return success, nil
}

func (rc *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "TTL", func() (interface{}, error) {
		return rc.client.TTL(ctx, prefixedKey).Result()
	})

	if err != nil {
		return 0, err
	}

	ttl, ok := result.(time.Duration)
	if !ok {
		return 0, ErrInvalidValue
	}

	return ttl, nil
}

func (rc *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "Incr", func() (interface{}, error) {
		return rc.client.Incr(ctx, prefixedKey).Result()
	})

	if err != nil {
		return 0, err
	}

	value, ok := result.(int64)
	if !ok {
		return 0, ErrInvalidValue
	}

	return value, nil
}

func (rc *RedisClient) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "IncrBy", func() (interface{}, error) {
		return rc.client.IncrBy(ctx, prefixedKey, value).Result()
	})

	if err != nil {
		return 0, err
	}

	newValue, ok := result.(int64)
	if !ok {
		return 0, ErrInvalidValue
	}

	return newValue, nil
}

func (rc *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "HGet", func() (interface{}, error) {
		return rc.client.HGet(ctx, prefixedKey, field).Result()
	})

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrKeyNotFound
		}
		return "", err
	}

	value, ok := result.(string)
	if !ok {
		return "", ErrInvalidValue
	}

	return value, nil
}

func (rc *RedisClient) HSet(ctx context.Context, key string, values ...interface{}) (int64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "HSet", func() (interface{}, error) {
		return rc.client.HSet(ctx, prefixedKey, values...).Result()
	})

	if err != nil {
		return 0, err
	}

	count, ok := result.(int64)
	if !ok {
		return 0, ErrInvalidValue
	}

	return count, nil
}

func (rc *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "HGetAll", func() (interface{}, error) {
		return rc.client.HGetAll(ctx, prefixedKey).Result()
	})

	if err != nil {
		return nil, err
	}

	values, ok := result.(map[string]string)
	if !ok {
		return nil, ErrInvalidValue
	}

	return values, nil
}

func (rc *RedisClient) LPush(ctx context.Context, key string, values ...interface{}) (int64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "LPush", func() (interface{}, error) {
		return rc.client.LPush(ctx, prefixedKey, values...).Result()
	})

	if err != nil {
		return 0, err
	}

	count, ok := result.(int64)
	if !ok {
		return 0, ErrInvalidValue
	}

	return count, nil
}

func (rc *RedisClient) RPop(ctx context.Context, key string) (string, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "RPop", func() (interface{}, error) {
		return rc.client.RPop(ctx, prefixedKey).Result()
	})

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrKeyNotFound
		}
		return "", err
	}

	value, ok := result.(string)
	if !ok {
		return "", ErrInvalidValue
	}

	return value, nil
}

func (rc *RedisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "LRange", func() (interface{}, error) {
		return rc.client.LRange(ctx, prefixedKey, start, stop).Result()
	})

	if err != nil {
		return nil, err
	}

	values, ok := result.([]string)
	if !ok {
		return nil, ErrInvalidValue
	}

	return values, nil
}

func (rc *RedisClient) Close() error {
	return rc.client.Close()
}

func (rc *RedisClient) Pipeline() redis.Pipeliner {
	return rc.client.Pipeline()
}

func (rc *RedisClient) TxPipeline() redis.Pipeliner {
	return rc.client.TxPipeline()
}

func (rc *RedisClient) Client() *redis.Client {
	return rc.client
}
