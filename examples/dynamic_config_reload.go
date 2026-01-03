//go:build example_dynamic_config || example_all
// +build example_dynamic_config example_all

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skolldire/go-engine/pkg/app"
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
		Build()

	if err != nil {
		fmt.Printf("failed to build application: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Dynamic Configuration Demo ===")
	fmt.Println("Application started with dynamic configuration.")
	fmt.Println("Modify config files to see automatic reload.")
	fmt.Println("Press Ctrl+C to exit.\n")

	monitorConfigChanges(ctx, engine)

	fmt.Println("\nstarting application...")
	if err := engine.Run(); err != nil {
		fmt.Printf("application error: %v\n", err)
		os.Exit(1)
	}
}

func ExampleDynamicConfigReload() {
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
		Build()

	if err != nil {
		fmt.Printf("failed to build application: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Dynamic Configuration Demo ===")
	fmt.Println("Application started with dynamic configuration.")
	fmt.Println("Modify config files to see automatic reload.")
	fmt.Println("Press Ctrl+C to exit.\n")

	monitorConfigChanges(ctx, engine)

	fmt.Println("\nstarting application...")
	if err := engine.Run(); err != nil {
		fmt.Printf("application error: %v\n", err)
		os.Exit(1)
	}
}

func monitorConfigChanges(ctx context.Context, engine *app.Engine) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				config := engine.GetConfig()
				if config != nil {
					fmt.Printf("[%s] Configuration check - Router Port: %s\n",
						time.Now().Format("15:04:05"),
						config.Router.Port)
				}

				flags := engine.GetFeatureFlags()
				if flags != nil {
					if flags.IsEnabled("enable_new_api") {
						fmt.Printf("[%s] Feature flag 'enable_new_api' is ENABLED\n",
							time.Now().Format("15:04:05"))
					}
				}
			}
		}
	}()
}

