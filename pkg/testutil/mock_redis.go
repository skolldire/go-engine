package testutil

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
)

// RedisStore is the interface that *redis.RedisClient satisfies implicitly.
// Type your dependencies against RedisStore instead of *redis.RedisClient
// to be able to swap in MockRedisClient during tests.
type RedisStore interface {
	Ping(ctx context.Context) error
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
	Del(ctx context.Context, keys ...string) (int64, error)
	Exists(ctx context.Context, keys ...string) (int64, error)
	Expire(ctx context.Context, key string, expiration time.Duration) (bool, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	Incr(ctx context.Context, key string) (int64, error)
	IncrBy(ctx context.Context, key string, value int64) (int64, error)
	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key string, values ...interface{}) (int64, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	LPush(ctx context.Context, key string, values ...interface{}) (int64, error)
	RPop(ctx context.Context, key string) (string, error)
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	SAdd(ctx context.Context, key string, members ...interface{}) (int64, error)
	SMembers(ctx context.Context, key string) ([]string, error)
	SIsMember(ctx context.Context, key string, member interface{}) (bool, error)
	SRem(ctx context.Context, key string, members ...interface{}) (int64, error)
	Close() error
}

// MockRedisClient implements RedisStore with testify/mock.
//
// Usage:
//
//	m := testutil.NewMockRedisClient()
//	m.On("Get", mock.Anything, "session:123").Return("value", nil)
//	defer m.AssertExpectations(t)
type MockRedisClient struct {
	mock.Mock
}

// NewMockRedisClient creates an empty MockRedisClient.
func NewMockRedisClient() *MockRedisClient { return &MockRedisClient{} }

func (m *MockRedisClient) Ping(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockRedisClient) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return m.Called(ctx, key, value, expiration).Error(0)
}

func (m *MockRedisClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	args := m.Called(ctx, key, value, expiration)
	return args.Bool(0), args.Error(1)
}

func (m *MockRedisClient) Del(ctx context.Context, keys ...string) (int64, error) {
	varArgs := []interface{}{ctx}
	for _, k := range keys {
		varArgs = append(varArgs, k)
	}
	args := m.Called(varArgs...)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	varArgs := []interface{}{ctx}
	for _, k := range keys {
		varArgs = append(varArgs, k)
	}
	args := m.Called(varArgs...)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRedisClient) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	args := m.Called(ctx, key, expiration)
	return args.Bool(0), args.Error(1)
}

func (m *MockRedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(time.Duration), args.Error(1)
}

func (m *MockRedisClient) Incr(ctx context.Context, key string) (int64, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRedisClient) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	args := m.Called(ctx, key, value)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	args := m.Called(ctx, key, field)
	return args.String(0), args.Error(1)
}

func (m *MockRedisClient) HSet(ctx context.Context, key string, values ...interface{}) (int64, error) {
	varArgs := []interface{}{ctx, key}
	varArgs = append(varArgs, values...)
	args := m.Called(varArgs...)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(map[string]string), args.Error(1)
}

func (m *MockRedisClient) LPush(ctx context.Context, key string, values ...interface{}) (int64, error) {
	varArgs := []interface{}{ctx, key}
	varArgs = append(varArgs, values...)
	args := m.Called(varArgs...)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRedisClient) RPop(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

func (m *MockRedisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	args := m.Called(ctx, key, start, stop)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockRedisClient) SAdd(ctx context.Context, key string, members ...interface{}) (int64, error) {
	varArgs := []interface{}{ctx, key}
	varArgs = append(varArgs, members...)
	args := m.Called(varArgs...)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	args := m.Called(ctx, key)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockRedisClient) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	args := m.Called(ctx, key, member)
	return args.Bool(0), args.Error(1)
}

func (m *MockRedisClient) SRem(ctx context.Context, key string, members ...interface{}) (int64, error) {
	varArgs := []interface{}{ctx, key}
	varArgs = append(varArgs, members...)
	args := m.Called(varArgs...)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRedisClient) Close() error {
	return m.Called().Error(0)
}

// SetupKeyNotFound configures the mock to return redis.Nil for the given key.
func (m *MockRedisClient) SetupKeyNotFound(key string) {
	m.On("Get", mock.Anything, key).Return("", redis.Nil)
}

// SetupGetReturn configures the mock to return value for the given key.
func (m *MockRedisClient) SetupGetReturn(key, value string) {
	m.On("Get", mock.Anything, key).Return(value, nil)
}

// SetupSetOK configures the mock to accept any Set call.
func (m *MockRedisClient) SetupSetOK() {
	m.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
}
