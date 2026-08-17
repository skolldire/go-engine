package app_test

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/app"
	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/health"
	pkgotel "github.com/skolldire/go-engine/pkg/telemetry/otel"
)

// readmeQuickStart mirrors the Quick Start block in README.md verbatim. It is
// never executed — it exists so the compiler fails the build when the README
// drifts from the real API, which is how JWKSEndpoint (a field that does not
// exist) survived in the documented example.
//
//nolint:unused // compile-time documentation guard, intentionally not called
func readmeQuickStart(myDBChecker, myRedisChecker health.Checker, usersHandler http.HandlerFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-sig; cancel() }()

	engine, err := app.NewAppBuilder().
		WithContext(ctx).
		WithDynamicConfig().
		WithOTEL(pkgotel.OTELConfig{
			ServiceName:      "my-service",
			ExporterEndpoint: "localhost:4317",
			Enabled:          true,
		}).
		WithInitialization().
		WithRouter().
		WithHealth(health.Config{Timeout: 5 * time.Second}).
		RegisterHealthChecker("postgres", myDBChecker).
		RegisterHealthChecker("redis", myRedisChecker).
		WithJWTAuth(router.JWTAuthConfig{
			JWKSURL:   "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX/.well-known/jwks.json",
			Issuer:    "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX",
			Audience:  "your-client-id",
			SkipPaths: []string{"/health", "/ping", "/live", "/ready"},
		}).
		Build()
	if err != nil {
		os.Exit(1)
	}

	engine.GetRouter().AddRoute("GET", "/users", usersHandler)

	if err := engine.Run(); err != nil {
		os.Exit(1)
	}
}

// TestReadmeQuickStartCompiles documents why the function above exists; the
// real assertion is performed by the compiler.
func TestReadmeQuickStartCompiles(t *testing.T) {
	t.Log("README Quick Start compiles against the current API")
}
