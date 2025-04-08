package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

func NewClient(cfg Config, log logger.Service) (*RedisClient, error) {
	timeoutDuration := cfg.Timeout
	if timeoutDuration == 0 {
		timeoutDuration = DefaultTimeout
	}

	dialTimeout := cfg.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = DefaultDialTimeout
	}

	readTimeout := cfg.ReadTimeout
	if readTimeout == 0 {
		readTimeout = DefaultReadTimeout
	}

	writeTimeout := cfg.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = DefaultWriteTimeout
	}

	poolSize := cfg.PoolSize
	if poolSize == 0 {
		poolSize = DefaultPoolSize
	}

	options := &redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		DB:           cfg.DB,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		PoolSize:     poolSize,
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

	if cfg.WithResilience {
		resilienceService := resilience.NewResilienceService(cfg.Resilience, log)
		rc.resilience = resilienceService
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	if err := rc.Ping(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	if rc.logging {
		log.Debug(ctx, "Conexión a Redis establecida correctamente",
			map[string]interface{}{
				"host":          cfg.Host,
				"port":          cfg.Port,
				"dial_timeout":  dialTimeout,
				"read_timeout":  readTimeout,
				"write_timeout": writeTimeout,
				"pool_size":     poolSize,
			})
	}

	return rc, nil
}

func (rc *RedisClient) KeyName(key string) string {
	if rc.keyPrefix == "" {
		return key
	}
	return fmt.Sprintf("%s:%s", rc.keyPrefix, key)
}

func (rc *RedisClient) ensureDefaultExpiration(expiration time.Duration) time.Duration {
	if expiration == 0 {
		return DefaultExpiration
	}
	return expiration
}

func (rc *RedisClient) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return context.WithCancel(ctx)
	}

	timeout := rc.client.Options().ReadTimeout
	if timeout <= 0 {
		timeout = DefaultReadTimeout
	}

	return context.WithTimeout(ctx, timeout)
}

func (rc *RedisClient) execute(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := rc.ensureContextWithTimeout(ctx)
	defer cancel()

	logFields := map[string]interface{}{"operation": operationName}

	if rc.resilience != nil {
		if rc.logging {
			rc.logger.Debug(ctx, fmt.Sprintf("Iniciando operación Redis con resiliencia: %s", operationName), logFields)
		}

		result, err := rc.resilience.Execute(ctx, operation)

		if err != nil && rc.logging {
			rc.logger.Error(ctx, fmt.Errorf("error en operación Redis: %w", err), logFields)
		} else if rc.logging {
			rc.logger.Debug(ctx, fmt.Sprintf("Operación Redis completada con resiliencia: %s", operationName), logFields)
		}

		return result, err
	}

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
	expiration = rc.ensureDefaultExpiration(expiration)

	_, err := rc.execute(ctx, "Set", func() (interface{}, error) {
		return rc.client.Set(ctx, prefixedKey, value, expiration).Result()
	})

	return err
}

func (rc *RedisClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	prefixedKey := rc.KeyName(key)
	expiration = rc.ensureDefaultExpiration(expiration)

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
	expiration = rc.ensureDefaultExpiration(expiration)

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

func (rc *RedisClient) ZAdd(ctx context.Context, key string, score float64, member interface{}) (int64, error) {
	prefixedKey := rc.KeyName(key)

	z := redis.Z{
		Score:  score,
		Member: member,
	}

	result, err := rc.execute(ctx, "ZAdd", func() (interface{}, error) {
		return rc.client.ZAdd(ctx, prefixedKey, z).Result()
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

func (rc *RedisClient) ZAddMulti(ctx context.Context, key string, members ...redis.Z) (int64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "ZAddMulti", func() (interface{}, error) {
		return rc.client.ZAdd(ctx, prefixedKey, members...).Result()
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

func (rc *RedisClient) ZScore(ctx context.Context, key string, member string) (float64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "ZScore", func() (interface{}, error) {
		return rc.client.ZScore(ctx, prefixedKey, member).Result()
	})

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, ErrKeyNotFound
		}
		return 0, err
	}

	score, ok := result.(float64)
	if !ok {
		return 0, ErrInvalidValue
	}

	return score, nil
}

func (rc *RedisClient) ZRem(ctx context.Context, key string, members ...interface{}) (int64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "ZRem", func() (interface{}, error) {
		return rc.client.ZRem(ctx, prefixedKey, members...).Result()
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

func (rc *RedisClient) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "ZRange", func() (interface{}, error) {
		return rc.client.ZRange(ctx, prefixedKey, start, stop).Result()
	})

	if err != nil {
		return nil, err
	}

	members, ok := result.([]string)
	if !ok {
		return nil, ErrInvalidValue
	}

	return members, nil
}

func (rc *RedisClient) SAdd(ctx context.Context, key string, members ...interface{}) (int64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "SAdd", func() (interface{}, error) {
		return rc.client.SAdd(ctx, prefixedKey, members...).Result()
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

func (rc *RedisClient) SAddWithExpire(ctx context.Context, key string, expiration time.Duration, members ...interface{}) (int64, error) {
	prefixedKey := rc.KeyName(key)
	expiration = rc.ensureDefaultExpiration(expiration)

	count, err := rc.execute(ctx, "SAddWithExpire", func() (interface{}, error) {
		count, err := rc.client.SAdd(ctx, prefixedKey, members...).Result()
		if err != nil {
			return 0, err
		}

		_, err = rc.client.Expire(ctx, prefixedKey, expiration).Result()
		if err != nil {
			return count, err
		}

		return count, nil
	})

	if err != nil {
		return 0, err
	}

	countVal, ok := count.(int64)
	if !ok {
		return 0, ErrInvalidValue
	}

	return countVal, nil
}

func (rc *RedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "SMembers", func() (interface{}, error) {
		return rc.client.SMembers(ctx, prefixedKey).Result()
	})

	if err != nil {
		return nil, err
	}

	members, ok := result.([]string)
	if !ok {
		return nil, ErrInvalidValue
	}

	return members, nil
}

func (rc *RedisClient) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "SIsMember", func() (interface{}, error) {
		return rc.client.SIsMember(ctx, prefixedKey, member).Result()
	})

	if err != nil {
		return false, err
	}

	isMember, ok := result.(bool)
	if !ok {
		return false, ErrInvalidValue
	}

	return isMember, nil
}

func (rc *RedisClient) SRem(ctx context.Context, key string, members ...interface{}) (int64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "SRem", func() (interface{}, error) {
		return rc.client.SRem(ctx, prefixedKey, members...).Result()
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

func (rc *RedisClient) SCard(ctx context.Context, key string) (int64, error) {
	prefixedKey := rc.KeyName(key)

	result, err := rc.execute(ctx, "SCard", func() (interface{}, error) {
		return rc.client.SCard(ctx, prefixedKey).Result()
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
