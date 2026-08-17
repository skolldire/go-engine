package client

import "context"

// Invocation describes the operation a client is about to run. It is the
// low-cardinality metadata every cross-cutting concern needs: which client, and
// which logical operation — not the arguments.
type Invocation struct {
	// Client is the service name, e.g. "REST", "SQS", "Redis".
	Client string
	// Operation is the logical operation, e.g. "GET /users/{id}" or
	// "SendMessage". It must be a template, never a resolved value, so metrics
	// and spans do not explode into one series per identifier.
	Operation string
}

// Handler executes one invocation. It is the seam every decorator wraps.
type Handler func(ctx context.Context, inv Invocation, op Operation) (any, error)

// Middleware adds one cross-cutting concern to a Handler.
//
// Logging, metrics, tracing and resilience are all Middleware. Before this
// existed, each of the twenty client packages re-implemented `if c.logging
// { ... }` around every call, which meant adding metrics or tracing would have
// meant editing all twenty again. A client now composes the concerns it wants
// and pays for nothing else.
type Middleware func(next Handler) Handler

// Chain composes middleware into one, applied outermost-first: the first
// argument sees the invocation before the second.
//
// Order is a real decision, not a detail. Logging outermost means one logical
// call produces one log line no matter how many attempts a retry decorator made
// underneath; resilience innermost means the breaker and retry observe the
// individual attempts they are meant to govern.
func Chain(mw ...Middleware) Middleware {
	return func(next Handler) Handler {
		for i := len(mw) - 1; i >= 0; i-- {
			if mw[i] != nil {
				next = mw[i](next)
			}
		}
		return next
	}
}

// baseHandler is the innermost Handler: it just runs the operation.
func baseHandler(ctx context.Context, _ Invocation, op Operation) (any, error) {
	return op(ctx)
}
