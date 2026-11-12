// Package repository holds the data access layer of the example service.
//
// The engine hands the repository already-built clients: it never constructs
// them itself, which is what keeps this layer testable against a fake.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Order is the domain record this service stores.
type Order struct {
	ID    string  `json:"id"`
	Item  string  `json:"item"`
	Total float64 `json:"total"`
}

// Cache is the subset of the Redis client this repository needs.
//
// Declaring the dependency as a narrow interface here, rather than importing
// the Redis provider, is what lets the use case be tested without a container
// and keeps the domain free of adapter types.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
}

// OrderRepository stores orders in the cache.
type OrderRepository struct {
	cache Cache
	ttl   time.Duration
}

// NewOrderRepository builds the repository over an already-connected cache.
func NewOrderRepository(cache Cache, ttl time.Duration) *OrderRepository {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &OrderRepository{cache: cache, ttl: ttl}
}

// Save stores an order.
func (r *OrderRepository) Save(ctx context.Context, order Order) error {
	encoded, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("encode order %s: %w", order.ID, err)
	}
	return r.cache.Set(ctx, r.key(order.ID), string(encoded), r.ttl)
}

// Find returns the order stored under id.
func (r *OrderRepository) Find(ctx context.Context, id string) (Order, error) {
	raw, err := r.cache.Get(ctx, r.key(id))
	if err != nil {
		return Order{}, fmt.Errorf("read order %s: %w", id, err)
	}

	var order Order
	if err := json.Unmarshal([]byte(raw), &order); err != nil {
		return Order{}, fmt.Errorf("decode order %s: %w", id, err)
	}
	return order, nil
}

func (r *OrderRepository) key(id string) string { return "order:" + id }
