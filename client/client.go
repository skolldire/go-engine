package client

import (
	"context"
	"errors"
	"fmt"
)

var ErrInvalidCall = errors.New("invalid client call")

type Invocation struct {
	Component string
	Operation string
}

type Operation[T any] func(context.Context) (T, error)
type Handler[T any] func(context.Context, Invocation, Operation[T]) (T, error)
type Middleware[T any] func(Handler[T]) Handler[T]

func Chain[T any](next Handler[T], middleware ...Middleware[T]) (Handler[T], error) {
	if next == nil {
		return nil, fmt.Errorf("%w: next is nil", ErrInvalidCall)
	}
	for i := len(middleware) - 1; i >= 0; i-- {
		if middleware[i] == nil {
			return nil, fmt.Errorf("%w: middleware[%d] is nil", ErrInvalidCall, i)
		}
		next = middleware[i](next)
		if next == nil {
			return nil, fmt.Errorf("%w: middleware[%d] returned nil handler", ErrInvalidCall, i)
		}
	}
	return next, nil
}

func Do[T any](ctx context.Context, info Invocation, operation Operation[T], middleware ...Middleware[T]) (T, error) {
	var zero T
	if ctx == nil {
		return zero, fmt.Errorf("%w: ctx is nil", ErrInvalidCall)
	}
	if operation == nil {
		return zero, fmt.Errorf("%w: operation is nil", ErrInvalidCall)
	}
	if size := len(info.Component); size == 0 || size > 128 {
		return zero, fmt.Errorf("%w: Component length %d bytes; expected 1..128", ErrInvalidCall, size)
	}
	if size := len(info.Operation); size == 0 || size > 128 {
		return zero, fmt.Errorf("%w: Operation length %d bytes; expected 1..128", ErrInvalidCall, size)
	}
	chain, err := Chain(func(ctx context.Context, _ Invocation, operation Operation[T]) (T, error) {
		return operation(ctx)
	}, middleware...)
	if err != nil {
		return zero, err
	}
	return chain(ctx, info, operation)
}
