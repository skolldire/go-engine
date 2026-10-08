package full_test

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/skolldire/go-engine/aws/provider/sqs"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/health"
	"github.com/skolldire/go-engine/pkg/router"
	presetfull "github.com/skolldire/go-engine/preset/full"
)

// readmeQuickStart mirrors the Quick Start block in README.md verbatim. It is
// never executed — it exists so the compiler fails the build when the README
// drifts from the real API, which is how JWKSEndpoint (a field that does not
// exist) survived in the documented example.
//
// It lives in the full preset because that is the only module that can see both
// the core and an adapter, which is what the documented example shows.
//
//nolint:unused // compile-time documentation guard, intentionally not called
func readmeQuickStart(usersHandler http.HandlerFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-sig; cancel() }()

	// The preset supplies router, health probes and every component the YAML
	// declares; anything after it refines that baseline.
	opts := append(presetfull.Options(),
		engine.WithHealthConfig(health.Config{Timeout: 5 * time.Second}),
		engine.WithMiddleware(func(r router.Service) {
			r.Use(router.JWTAuth(router.JWTAuthConfig{
				JWKSURL:   "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX/.well-known/jwks.json",
				Issuer:    "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX",
				Audience:  "your-client-id",
				SkipPaths: []string{"/health", "/ping", "/live", "/ready"},
			}))
		}),
	)

	eng, err := engine.New(ctx, opts...)
	if err != nil {
		os.Exit(1)
	}
	defer func() { _ = eng.Close(ctx) }()

	eng.Router().AddRoute("GET", "/users", usersHandler)

	// Typed retrieval replaces the 39 getters: the type travels with the caller.
	orders, err := sqs.From(eng, "orders")
	if err != nil {
		os.Exit(1)
	}
	_ = orders

	if err := eng.Run(ctx); err != nil {
		os.Exit(1)
	}
}

// TestReadmeQuickStartCompiles documents why the function above exists; the
// real assertion is performed by the compiler.
func TestReadmeQuickStartCompiles(t *testing.T) {
	t.Log("README Quick Start compiles against the current API")
}
