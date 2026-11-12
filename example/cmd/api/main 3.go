// Command api is a microservice built with go-engine.
//
// It exists for two reasons that pull in the same direction: it is the worked
// example of how to wire the library, and it is the check that the library
// still works when someone actually implements it. A change that compiles and
// passes the library's own tests but makes an application awkward to build
// shows up here first.
//
// The wiring is deliberately explicit. Every component is registered by name,
// so what the service depends on can be read off this file rather than inferred
// from a configuration file somewhere else.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	redisprovider "github.com/skolldire/go-engine/database/redis/provider/redis"
	"github.com/skolldire/go-engine/example/internal/handler"
	"github.com/skolldire/go-engine/example/internal/repository"
	"github.com/skolldire/go-engine/example/internal/usecase"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/router"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// A cancelled context is the shutdown signal: the engine stops the server,
	// runs the shutdown hooks and releases every component in LIFO order.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	eng, err := buildEngine(ctx)
	if err != nil {
		return fmt.Errorf("building the engine: %w", err)
	}

	// Close is registered as a router shutdown hook, so this defer only matters
	// when Run is never reached. It is idempotent.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = eng.Close(shutdownCtx)
	}()

	if err := registerRoutes(eng); err != nil {
		return err
	}

	return eng.Run(ctx)
}

// buildEngine assembles the service. Options are declarative: the engine
// applies them in dependency order, so this list has no ordering rules to
// remember.
func buildEngine(ctx context.Context) (*engine.Engine, error) {
	return engine.New(ctx,
		// Configuration comes from config/application.yaml, plus the overlay
		// named by the SCOPE environment variable when one is set.
		engine.WithConfigFiles(configFiles()...),

		engine.WithRouter(),
		engine.WithHealth(),

		// Each adapter is opted into explicitly. A section in the YAML with no
		// provider registered here builds nothing, which is what keeps a service
		// from paying for what it does not use.
		engine.WithProvider(redisprovider.New("cache")),
	)
}

// configFiles returns the files to merge, later ones overriding earlier.
func configFiles() []string {
	files := []string{"application"}
	if scope := os.Getenv("SCOPE"); scope != "" {
		files = append(files, "application-"+scope)
	}
	return files
}

// registerRoutes builds the application layers over the engine's components and
// mounts them.
func registerRoutes(eng *engine.Engine) error {
	// Retrieval is typed: asking for a component that was never registered, or
	// with the wrong type, is an error naming what is available.
	cache, err := redisprovider.From(eng, "cache")
	if err != nil {
		return fmt.Errorf("resolving the cache: %w", err)
	}

	orders := repository.NewOrderRepository(cache, time.Hour)
	orderHandler := handler.NewOrder(
		usecase.NewCreateOrder(orders),
		usecase.NewGetOrder(orders),
	)

	var r router.Service = eng.Router()
	orderHandler.Register(r)

	return nil
}
