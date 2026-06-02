# go-engine/database/redis

Redis client for go-engine built on [go-redis/v9](https://github.com/redis/go-redis).

```bash
go get github.com/skolldire/go-engine/database/redis
```

---

## Configuration

```yaml
# Single instance (legacy)
redis:
  host: "localhost"
  port: 6379
  password: ""
  db: 0
  prefix: "app:"
  enable_logging: true

# Multi-instance (preferred)
redis_clients:
  - cache:
      host: "localhost"
      port: 6379
      db: 0
      prefix: "cache:"
      pool_size: 10
      timeout: 30s
      dial_timeout: 5s
      read_timeout: 3s
      write_timeout: 3s
  - session:
      host: "localhost"
      port: 6379
      db: 1
      prefix: "sess:"
```

Resilience (retry + circuit breaker):

```yaml
redis_clients:
  - cache:
      with_resilience: true
      resilience:
        retry_config:
          max_retries: 3
          initial_delay: 100ms
          max_delay: 1s
        circuit_breaker_config:
          name: "redis-cache"
          max_requests: 5
          timeout: 30s
```

---

## Usage

```go
rc := engine.GetRedisClientByName("cache")
// or legacy: engine.GetRedisClient()

// Strings
err := rc.Set(ctx, "user:42", data, time.Hour)
val, err := rc.Get(ctx, "user:42")
ok, err := rc.SetNX(ctx, "lock:42", "1", 30*time.Second) // set if not exists
n, err := rc.Del(ctx, "user:42", "user:43")

// TTL / expiry
ok, err := rc.Expire(ctx, "user:42", time.Hour)
ttl, err := rc.TTL(ctx, "user:42")
exists, err := rc.Exists(ctx, "user:42")

// Counters
n, err := rc.Incr(ctx, "counter:pageviews")
n, err = rc.IncrBy(ctx, "counter:downloads", 5)

// Hashes
err = rc.HSet(ctx, "user:42", "name", "John", "score", 9.5)
val, err = rc.HGet(ctx, "user:42", "name")
all, err := rc.HGetAll(ctx, "user:42")

// Lists (queue pattern)
n, err = rc.LPush(ctx, "jobs", job1, job2)
item, err := rc.RPop(ctx, "jobs")
items, err := rc.LRange(ctx, "jobs", 0, -1)

// Sets
n, err = rc.SAdd(ctx, "online-users", "u1", "u2")
n, err = rc.SAddWithExpire(ctx, "active-sessions", time.Hour, sessionID)
members, err := rc.SMembers(ctx, "online-users")
isMember, err := rc.SIsMember(ctx, "online-users", "u1")
n, err = rc.SRem(ctx, "online-users", "u1")
count, err := rc.SCard(ctx, "online-users")

// Sorted sets (leaderboards)
n, err = rc.ZAdd(ctx, "scores", 9.5, "student:42")
n, err = rc.ZAddMulti(ctx, "scores", redis.Z{Score: 9.5, Member: "s1"}, redis.Z{Score: 8.0, Member: "s2"})
score, err := rc.ZScore(ctx, "scores", "student:42")
ranking, err := rc.ZRange(ctx, "scores", 0, 9)  // top 10
n, err = rc.ZRem(ctx, "scores", "student:42")

// Health check
err = rc.Ping(ctx)

// Pipelines (batch commands)
pipe := rc.Pipeline()
// or: pipe := rc.TxPipeline() for atomic pipeline
```

**Errors:** `redis.ErrKeyNotFound`, `redis.ErrInvalidValue`, `redis.ErrConnection`.

Keys are automatically prefixed with `Config.Prefix`. Use `rc.KeyName("mykey")` to see the full key name.
