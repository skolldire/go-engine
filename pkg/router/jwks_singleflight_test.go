package router

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingJWKSServer serves a valid JWKS, counts requests and holds each one
// open for the given delay so concurrency is observable.
func countingJWKSServer(t *testing.T, key *rsa.PrivateKey, delay time.Duration) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var calls atomic.Int64
	pub := &key.PublicKey
	nBytes := pub.N.Bytes()
	eBytes := big.NewInt(int64(pub.E)).Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(delay)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kid": testKID,
				"kty": "RSA",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(nBytes),
				"e":   base64.RawURLEncoding.EncodeToString(eBytes),
			}},
		})
	}))
	t.Cleanup(srv.Close)

	return srv, &calls
}

// TestJWKSCache_ConcurrentRefreshCollapses is the regression test for getKey
// holding the write lock across the JWKS HTTP fetch: during a key rotation
// every concurrent request serialised behind one network round trip. With
// singleflight the fetch happens once and all callers share the result.
func TestJWKSCache_ConcurrentRefreshCollapses(t *testing.T) {
	key := generateTestKey(t)
	srv, calls := countingJWKSServer(t, key, 200*time.Millisecond)

	cache := &jwksCache{
		endpoint:   srv.URL,
		ttl:        time.Hour,
		keys:       make(map[string]*rsa.PublicKey),
		httpClient: srv.Client(),
		now:        time.Now,
	}

	const concurrent = 20
	var wg sync.WaitGroup
	results := make([]error, concurrent)

	start := time.Now()
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := cache.getKey(context.Background(), testKID)
			results[i] = err
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	for i, err := range results {
		require.NoError(t, err, "goroutine %d", i)
	}

	assert.Equal(t, int64(1), calls.Load(),
		"concurrent cache misses must collapse into a single JWKS fetch")
	assert.Less(t, elapsed, 1*time.Second,
		"callers must share one fetch instead of serialising behind the write lock")
}

// TestJWKSCache_ServesFromCacheAfterFetch verifies the happy path still caches.
func TestJWKSCache_ServesFromCacheAfterFetch(t *testing.T) {
	key := generateTestKey(t)
	srv, calls := countingJWKSServer(t, key, 0)

	cache := &jwksCache{
		endpoint:   srv.URL,
		ttl:        time.Hour,
		keys:       make(map[string]*rsa.PublicKey),
		httpClient: srv.Client(),
		now:        time.Now,
	}

	for i := 0; i < 5; i++ {
		k, err := cache.getKey(context.Background(), testKID)
		require.NoError(t, err)
		require.NotNil(t, k)
	}

	assert.Equal(t, int64(1), calls.Load(), "subsequent hits must be served from cache")
}

// TestJWKSCache_UnknownKidReportsError guards the not-found path after the
// refactor moved the map lookup out from under the write lock.
func TestJWKSCache_UnknownKidReportsError(t *testing.T) {
	key := generateTestKey(t)
	srv, _ := countingJWKSServer(t, key, 0)

	cache := &jwksCache{
		endpoint:   srv.URL,
		ttl:        time.Hour,
		keys:       make(map[string]*rsa.PublicKey),
		httpClient: srv.Client(),
		now:        time.Now,
	}

	k, err := cache.getKey(context.Background(), "unknown-kid")
	assert.Nil(t, k)
	assert.ErrorContains(t, err, "unknown-kid")
}
