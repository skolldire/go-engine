package redis

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	baseclient "github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// testRedisAddr / testRedisHost / testRedisPort point at a TCP port that is
// reserved and immediately closed, so nothing listens there for the duration
// of the test run. This keeps the suite hermetic: every operation fails fast
// with a connection error regardless of whether the developer happens to have
// a real Redis listening on localhost:6379.
var (
	testRedisAddr string
	testRedisHost string
	testRedisPort int
)

func TestMain(m *testing.M) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("could not reserve a dead port for redis tests: " + err.Error())
	}
	testRedisAddr = l.Addr().String()
	host, portStr, _ := net.SplitHostPort(testRedisAddr)
	testRedisHost = host
	testRedisPort, _ = strconv.Atoi(portStr)
	// Close so nothing listens on the reserved port during the tests.
	_ = l.Close()

	os.Exit(m.Run())
}

// newDeadRedisClient returns a go-redis client pointed at the reserved dead
// port, with retries disabled and short timeouts so every command fails fast
// with a connection error instead of retrying for seconds.
func newDeadRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         testRedisAddr,
		MaxRetries:   -1,
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	})
}

type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]any) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]any) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]any) {
	m.Called(ctx, err, fields)
}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]any) {}
func (m *mockLogger) WrapError(err error, msg string) error                            { return err }
func (m *mockLogger) WithField(key string, value any) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]any) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                              { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                   { return nil }

func TestNewClient_DefaultValues(t *testing.T) {
	cfg := Config{
		Host:          testRedisHost,
		Port:          testRedisPort,
		DB:            0,
		Password:      "",
		Timeout:       0,
		DialTimeout:   200 * time.Millisecond,
		ReadTimeout:   0,
		WriteTimeout:  0,
		PoolSize:      0,
		Prefix:        "",
		EnableLogging: false,
	}
	log := &mockLogger{}

	// The reserved port has no listener, so the Ping in NewClient must fail.
	// require stops the test here to avoid dereferencing a nil error below.
	_, err := NewClient(cfg, log)
	require.Error(t, err)
	// And we can verify the config is processed correctly
	assert.Contains(t, err.Error(), "connection")
}

func TestNewClient_WithPassword(t *testing.T) {
	cfg := Config{
		Host:        testRedisHost,
		Port:        testRedisPort,
		DialTimeout: 200 * time.Millisecond,
		Password:    "password123",
	}
	log := &mockLogger{}

	_, err := NewClient(cfg, log)
	require.Error(t, err)
}

func TestNewClient_WithPrefix(t *testing.T) {
	cfg := Config{
		Host:        testRedisHost,
		Port:        testRedisPort,
		DialTimeout: 200 * time.Millisecond,
		Prefix:      "app:",
	}
	log := &mockLogger{}

	_, err := NewClient(cfg, log)
	require.Error(t, err)
}

func TestNewClient_WithResilience(t *testing.T) {
	cfg := Config{
		Host:           testRedisHost,
		Port:           testRedisPort,
		DialTimeout:    200 * time.Millisecond,
		WithResilience: true,
		Resilience: resilience.Config{
			RetryConfig: &retry_backoff.Config{
				MaxRetries: 3,
			},
			CircuitBreakerConfig: &circuit_breaker.Config{
				Name: "test-cb",
			},
		},
	}
	log := &mockLogger{}

	// Configure mock to handle Debug calls during retry logic (may be called multiple times)
	log.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	// Configure mock to handle Error calls at the end of retries
	log.On("Error", mock.Anything, mock.Anything, mock.Anything).Return().Maybe()

	// The reserved port has no listener, so the connection must fail.
	_, err := NewClient(cfg, log)
	require.Error(t, err)
}

func TestRedisClient_KeyName(t *testing.T) {
	log := &mockLogger{}

	// Create a client that will fail connection but we can test KeyName
	// Note: KeyName adds ":" between prefix and key, so prefix should not include ":"
	client := &RedisClient{
		keyPrefix:  "app",
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
	}

	assert.Equal(t, "app:test-key", client.KeyName("test-key"))
}

func TestRedisClient_KeyName_NoPrefix(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		keyPrefix:  "",
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
	}

	assert.Equal(t, "test-key", client.KeyName("test-key"))
}

func TestRedisClient_EnsureDefaultExpiration(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
	}

	assert.Equal(t, DefaultExpiration, client.ensureDefaultExpiration(0))
	assert.Equal(t, 5*time.Minute, client.ensureDefaultExpiration(5*time.Minute))
}

func TestRedisClient_EnsureContextWithTimeout(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	newCtx, cancel := client.ContextWithTimeout(ctx)
	assert.NotNil(t, newCtx)
	assert.NotNil(t, cancel)
	cancel()
}

func TestRedisClient_EnsureContextWithTimeout_WithDeadline(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		client:     newDeadRedisClient(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	newCtx, cancelFunc := client.ContextWithTimeout(ctx)
	assert.NotNil(t, newCtx)
	assert.NotNil(t, cancelFunc)
	cancelFunc()
}

func TestRedisClient_Get_KeyNotFound(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	// This will fail without a real Redis connection
	_, err := client.Get(ctx, "nonexistent-key")
	assert.Error(t, err)
}

func TestRedisClient_Set(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	err := client.Set(ctx, "test-key", "test-value", 0)
	assert.Error(t, err)
}

func TestRedisClient_SetNX(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.SetNX(ctx, "test-key", "test-value", 0)
	assert.Error(t, err)
}

func TestRedisClient_Del(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.Del(ctx, "key1", "key2")
	assert.Error(t, err)
}

func TestRedisClient_Exists(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.Exists(ctx, "key1", "key2")
	assert.Error(t, err)
}

func TestRedisClient_Expire(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.Expire(ctx, "test-key", 60*time.Second)
	assert.Error(t, err)
}

func TestRedisClient_TTL(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.TTL(ctx, "test-key")
	assert.Error(t, err)
}

func TestRedisClient_Incr(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.Incr(ctx, "counter-key")
	assert.Error(t, err)
}

func TestRedisClient_IncrBy(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.IncrBy(ctx, "counter-key", 5)
	assert.Error(t, err)
}

func TestRedisClient_HGet(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.HGet(ctx, "hash-key", "field")
	assert.Error(t, err)
}

func TestRedisClient_HSet(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.HSet(ctx, "hash-key", "field", "value")
	assert.Error(t, err)
}

func TestRedisClient_HGetAll(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.HGetAll(ctx, "hash-key")
	assert.Error(t, err)
}

func TestRedisClient_LPush(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.LPush(ctx, "list-key", "value1", "value2")
	assert.Error(t, err)
}

func TestRedisClient_RPop(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.RPop(ctx, "list-key")
	assert.Error(t, err)
}

func TestRedisClient_LRange(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.LRange(ctx, "list-key", 0, -1)
	assert.Error(t, err)
}

func TestRedisClient_ZAdd(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.ZAdd(ctx, "zset-key", 1.0, "member")
	assert.Error(t, err)
}

func TestRedisClient_ZAddMulti(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	members := []redis.Z{
		{Score: 1.0, Member: "member1"},
		{Score: 2.0, Member: "member2"},
	}
	_, err := client.ZAddMulti(ctx, "zset-key", members...)
	assert.Error(t, err)
}

func TestRedisClient_ZScore(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.ZScore(ctx, "zset-key", "member")
	assert.Error(t, err)
}

func TestRedisClient_ZRem(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.ZRem(ctx, "zset-key", "member1", "member2")
	assert.Error(t, err)
}

func TestRedisClient_ZRange(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.ZRange(ctx, "zset-key", 0, -1)
	assert.Error(t, err)
}

func TestRedisClient_SAdd(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.SAdd(ctx, "set-key", "member1", "member2")
	assert.Error(t, err)
}

func TestRedisClient_SAddWithExpire(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.SAddWithExpire(ctx, "set-key", 60*time.Second, "member1", "member2")
	assert.Error(t, err)
}

func TestRedisClient_SMembers(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.SMembers(ctx, "set-key")
	assert.Error(t, err)
}

func TestRedisClient_SIsMember(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.SIsMember(ctx, "set-key", "member")
	assert.Error(t, err)
}

func TestRedisClient_SRem(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.SRem(ctx, "set-key", "member1", "member2")
	assert.Error(t, err)
}

func TestRedisClient_SCard(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	_, err := client.SCard(ctx, "set-key")
	assert.Error(t, err)
}

func TestRedisClient_Close(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		client:     newDeadRedisClient(),
	}

	err := client.Close()
	assert.NoError(t, err)
}

func TestRedisClient_Pipeline(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		client:     newDeadRedisClient(),
	}

	pipeline := client.Pipeline()
	assert.NotNil(t, pipeline)
}

func TestRedisClient_TxPipeline(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		client:     newDeadRedisClient(),
	}

	pipeline := client.TxPipeline()
	assert.NotNil(t, pipeline)
}

func TestRedisClient_Client(t *testing.T) {
	log := &mockLogger{}
	redisClient := newDeadRedisClient()
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		client:     redisClient,
	}

	assert.Equal(t, redisClient, client.Client())
}

func TestRedisClient_Get_WithKeyNotFoundError(t *testing.T) {
	log := &mockLogger{}
	client := &RedisClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, log, "Redis"),
		keyPrefix:  "",
		client:     newDeadRedisClient(),
	}

	ctx := context.Background()
	// This will fail without a real Redis connection
	_, err := client.Get(ctx, "nonexistent-key")
	assert.Error(t, err)
	// In a real scenario with redis.Nil, it should return ErrKeyNotFound
	// but without connection, we get connection error
}
