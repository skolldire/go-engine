package testutil

import (
	"context"

	"github.com/skolldire/go-engine/pkg/app/router"
)

// NewTestContext creates a context.Context with JWT Claims injected,
// simulating a request that has passed through JWTMiddleware.
// Use in tests of handlers or use-cases that call router.ClaimsFromContext.
//
// Usage:
//
//	ctx := testutil.NewTestContext(&router.Claims{
//	    Sub:    "user-123",
//	    Email:  "test@example.com",
//	    Groups: []string{"teachers"},
//	})
//	result, err := handler.Handle(ctx, req)
func NewTestContext(claims *router.Claims) context.Context {
	return router.InjectClaimsForTest(context.Background(), claims)
}

// NewEmptyTestContext returns a plain background context with no claims.
// Use to simulate unauthenticated requests or public routes (SkipPaths).
func NewEmptyTestContext() context.Context {
	return context.Background()
}
