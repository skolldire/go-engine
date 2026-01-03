//go:build example_complete || example_all
// +build example_complete example_all

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skolldire/go-engine/pkg/app"
	"github.com/skolldire/go-engine/pkg/app/router"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		fmt.Println("shutdown signal received")
		cancel()
	}()

	engine, err := app.NewAppBuilder().
		WithContext(ctx).
		WithDynamicConfig().
		WithInitialization().
		WithRouter().
		WithMiddleware(setupCustomMiddleware).
		WithGracefulShutdown().
		Build()

	if err != nil {
		fmt.Printf("failed to build application: %v\n", err)
		os.Exit(1)
	}

	if errs := engine.GetErrors(); len(errs) > 0 {
		fmt.Printf("initialization errors: %v\n", errs)
		os.Exit(1)
	}

	setupRoutes(engine)

	fmt.Println("starting application...")
	if err := engine.Run(); err != nil {
		fmt.Printf("application error: %v\n", err)
		os.Exit(1)
	}
}

func setupCustomMiddleware(r router.Service) {
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, req)
			duration := time.Since(start)
			fmt.Printf("request completed in %v\n", duration)
		})
	})
}

func setupRoutes(engine *app.Engine) {
	r := engine.GetRouter()

	r.AddRoute("GET", "/health", healthCheckHandler(engine))
	r.AddRoute("GET", "/feature-flags", featureFlagsHandler(engine))
	r.AddRoute("GET", "/clients", clientsInfoHandler(engine))
}

func healthCheckHandler(engine *app.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
	}
}

func featureFlagsHandler(engine *app.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flags := engine.GetFeatureFlags()
		if flags == nil {
			http.Error(w, "feature flags not available", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"flags":%v}`, flags.GetAll())
	}
}

func clientsInfoHandler(engine *app.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := map[string]interface{}{
			"rest_clients":    len(engine.RestClients),
			"grpc_clients":    len(engine.GrpcClients),
			"sqs_clients":     len(engine.SQSClients),
			"sns_clients":     len(engine.SNSClients),
			"dynamo_clients":  len(engine.DynamoDBClients),
			"redis_clients":   len(engine.RedisClients),
			"sql_connections": len(engine.SQLConnections),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"clients":%v}`, info)
	}
}
