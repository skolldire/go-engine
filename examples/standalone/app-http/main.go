// Command app-http demonstrates explicit HTTP assembly without engine or drivers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/skolldire/go-engine/health"
	"github.com/skolldire/go-engine/logging"
	"github.com/skolldire/go-engine/presets/httpapi"
	"github.com/skolldire/go-engine/router"
	routererrors "github.com/skolldire/go-engine/router/errors"
	"github.com/skolldire/go-engine/router/middleware"
)

func main() {
	api := flag.String("api-listen", "127.0.0.1:8080", "API listener")
	ops := flag.String("operations-listen", "127.0.0.1:8081", "internal operations listener")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app, err := build(*api, *ops, os.Stdout)
	if err == nil {
		err = app.Run(ctx)
	}
	// Output is borrowed stdout and the store is memory-only: nothing to close.
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func build(api, ops string, output io.Writer) (*httpapi.Application, error) {
	return buildWithStore(api, ops, output, &memoryStore{orders: make(map[string]order)})
}
func buildWithStore(api, ops string, output io.Writer, store orderStore) (*httpapi.Application, error) {
	log, logErr := logging.New(logging.DefaultConfig(), logging.Dependencies{Output: output})
	reg, healthErr := health.New(health.DefaultConfig())
	responder, responseErr := routererrors.NewResponder(routererrors.DefaultConfig(), routererrors.Dependencies{Classify: classify, RequestID: func(ctx context.Context) string { id, _ := middleware.RequestIDFrom(ctx); return id }})
	if err := errors.Join(logErr, healthErr, responseErr); err != nil {
		return nil, err
	}
	if err := reg.Register("memory", health.CheckerFunc(func(context.Context) error { return nil })); err != nil {
		return nil, err
	}
	cfg := httpapi.DefaultConfig()
	cfg.Router = router.Config{AllowedHosts: []string{"localhost", "127.0.0.1"}}
	cfg.OperationsRouter = cfg.Router
	cfg.Server.Listen = api
	cfg.OperationsServer.Listen = ops
	return httpapi.New(cfg, httpapi.Dependencies{Routes: routes(store, responder, log), Health: reg, OnPanic: func(w http.ResponseWriter, r *http.Request, value any) {
		log.ErrorContext(r.Context(), "business panic recovered")
		if err := responder.Write(r.Context(), w, errors.New("private panic cause")); err != nil {
			log.ErrorContext(r.Context(), "response write failed")
		}
	}})
}
