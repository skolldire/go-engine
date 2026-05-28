package health

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// redisPinger is satisfied by *redis.RedisClient (pkg/database/redis).
type redisPinger interface {
	Ping(ctx context.Context) error
}

// sqlPinger is satisfied by *gormsql.DBClient (database/sql sub-module).
type sqlPinger interface {
	Ping(ctx context.Context) error
}

// ── RedisChecker ──────────────────────────────────────────────────────────────

// RedisChecker verifies Redis connectivity using the project's RedisClient.
type RedisChecker struct {
	client redisPinger
}

// NewRedisChecker creates a Checker backed by any redisPinger.
// Pass *redis.RedisClient from pkg/database/redis directly.
func NewRedisChecker(client redisPinger) *RedisChecker {
	return &RedisChecker{client: client}
}

func (c *RedisChecker) Check(ctx context.Context) error {
	if err := c.client.Ping(ctx); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

// ── SQLChecker ────────────────────────────────────────────────────────────────

// SQLChecker verifies SQL database connectivity via Ping.
// Pass *gormsql.DBClient from the database/sql sub-module directly.
type SQLChecker struct {
	client sqlPinger
}

// NewSQLChecker creates a Checker backed by any sqlPinger.
func NewSQLChecker(client sqlPinger) *SQLChecker {
	return &SQLChecker{client: client}
}

func (c *SQLChecker) Check(ctx context.Context) error {
	if err := c.client.Ping(ctx); err != nil {
		return fmt.Errorf("sql ping: %w", err)
	}
	return nil
}

// ── HTTPChecker ───────────────────────────────────────────────────────────────

// HTTPChecker verifies an external URL by issuing a GET request.
// It considers any 5xx status code a failure.
type HTTPChecker struct {
	url     string
	client  *http.Client
}

// NewHTTPChecker creates a Checker that GETs url.
// If timeout is 0 it defaults to DefaultTimeout.
func NewHTTPChecker(url string, timeout time.Duration) *HTTPChecker {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &HTTPChecker{
		url:    url,
		client: &http.Client{Timeout: timeout},
	}
}

func (c *HTTPChecker) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
