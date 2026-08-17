package testutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/skolldire/go-engine/database/redis/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ── MockRedisClient ───────────────────────────────────────────────────────────

func TestMockRedisClient_SetAndGet(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.On("Set", mock.Anything, "key1", "value1", time.Minute).Return(nil)
	m.On("Get", mock.Anything, "key1").Return("value1", nil)
	defer m.AssertExpectations(t)

	err := m.Set(context.Background(), "key1", "value1", time.Minute)
	assert.NoError(t, err)

	val, err := m.Get(context.Background(), "key1")
	assert.NoError(t, err)
	assert.Equal(t, "value1", val)
}

func TestMockRedisClient_SetupKeyNotFound(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.SetupKeyNotFound("missing")
	defer m.AssertExpectations(t)

	_, err := m.Get(context.Background(), "missing")
	assert.Error(t, err)
}

func TestMockRedisClient_SetupGetReturn(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.SetupGetReturn("key", "cached-value")
	defer m.AssertExpectations(t)

	val, err := m.Get(context.Background(), "key")
	assert.NoError(t, err)
	assert.Equal(t, "cached-value", val)
}

func TestMockRedisClient_SetupSetOK(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.SetupSetOK()
	defer m.AssertExpectations(t)

	err := m.Set(context.Background(), "any", "value", time.Hour)
	assert.NoError(t, err)
}

func TestMockRedisClient_Ping(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.On("Ping", mock.Anything).Return(nil)
	defer m.AssertExpectations(t)

	assert.NoError(t, m.Ping(context.Background()))
}

func TestMockRedisClient_Del(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.On("Del", mock.Anything, "k1", "k2").Return(int64(2), nil)
	defer m.AssertExpectations(t)

	n, err := m.Del(context.Background(), "k1", "k2")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestMockRedisClient_HGetAll(t *testing.T) {
	m := testutil.NewMockRedisClient()
	expected := map[string]string{"field": "value"}
	m.On("HGetAll", mock.Anything, "hash-key").Return(expected, nil)
	defer m.AssertExpectations(t)

	result, err := m.HGetAll(context.Background(), "hash-key")
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestMockRedisClient_SMembers(t *testing.T) {
	m := testutil.NewMockRedisClient()
	m.On("SMembers", mock.Anything, "set-key").Return([]string{"a", "b"}, nil)
	defer m.AssertExpectations(t)

	members, err := m.SMembers(context.Background(), "set-key")
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, members)
}
