// Package usecase holds the application logic of the example service.
package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/skolldire/go-engine/example/internal/repository"
)

// ErrInvalidOrder is returned for a request that cannot be accepted.
var ErrInvalidOrder = errors.New("invalid order")

// Orders is the repository this use case needs.
type Orders interface {
	Save(ctx context.Context, order repository.Order) error
	Find(ctx context.Context, id string) (repository.Order, error)
}

// CreateOrder validates and stores an order.
type CreateOrder struct {
	orders Orders
}

// NewCreateOrder builds the use case.
func NewCreateOrder(orders Orders) *CreateOrder {
	return &CreateOrder{orders: orders}
}

// Execute stores the order after validating it.
func (c *CreateOrder) Execute(ctx context.Context, order repository.Order) error {
	if order.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidOrder)
	}
	if order.Item == "" {
		return fmt.Errorf("%w: item is required", ErrInvalidOrder)
	}
	if order.Total <= 0 {
		return fmt.Errorf("%w: total must be positive", ErrInvalidOrder)
	}
	return c.orders.Save(ctx, order)
}

// GetOrder returns a stored order.
type GetOrder struct {
	orders Orders
}

// NewGetOrder builds the use case.
func NewGetOrder(orders Orders) *GetOrder { return &GetOrder{orders: orders} }

// Execute returns the order stored under id.
func (g *GetOrder) Execute(ctx context.Context, id string) (repository.Order, error) {
	if id == "" {
		return repository.Order{}, fmt.Errorf("%w: id is required", ErrInvalidOrder)
	}
	return g.orders.Find(ctx, id)
}
