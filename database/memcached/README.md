# go-engine/database/memcached

Memcached client for go-engine built on [bradfitz/gomemcache](https://github.com/bradfitz/gomemcache).

```bash
go get github.com/skolldire/go-engine/database/memcached
```

---

## Configuration

```yaml
memcached_clients:
  - cache:
      servers:
        - "localhost:11211"
        - "localhost:11212"
      timeout: 5s
      max_idle_conns: 2
      prefix: "app:"
      enable_logging: true
```

---

## Usage

```go
mc := engine.GetMemcachedClientByName("cache")

// Set / Get
err := mc.Set(ctx, "session:abc", []byte(`{"user":"john"}`), time.Hour)
data, err := mc.Get(ctx, "session:abc")       // ErrKeyNotFound if missing

// Conditional set
err = mc.Add(ctx, "lock:resource", []byte("1"), 30*time.Second)     // only if not exists
err = mc.Replace(ctx, "session:abc", newData, time.Hour)            // only if exists

// Counters (value must be numeric string, e.g. "0")
err = mc.Set(ctx, "counter:views", []byte("0"), 0)
newVal, err := mc.Increment(ctx, "counter:views", 1)
newVal, err = mc.Decrement(ctx, "counter:views", 1)

// Batch read
results, err := mc.GetMulti(ctx, []string{"session:abc", "session:def"})
// missing keys are not in the returned map

// Delete / flush
err = mc.Delete(ctx, "session:abc")
err = mc.FlushAll(ctx)                // removes ALL keys from ALL servers

// Key name with prefix
fullKey := mc.KeyName("session:abc")  // → "app:session:abc"
```

**Errors:** `memcached.ErrKeyNotFound`, `memcached.ErrInvalidValue`, `memcached.ErrConnection`.

Keys are automatically prefixed with `Config.Prefix`. Default expiration when passing `0` is 24 hours.
