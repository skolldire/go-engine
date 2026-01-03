//go:build example_feature_flags || example_all
// +build example_feature_flags example_all

package main

import (
	"context"
	"fmt"

	"github.com/skolldire/go-engine/pkg/app"
)

func main() {
	ctx := context.Background()

	engine, err := app.NewAppBuilder().
		WithContext(ctx).
		WithDynamicConfig().
		WithInitialization().
		WithRouter().
		Build()

	if err != nil {
		panic(err)
	}

	demonstrateFeatureFlags(ctx, engine)
}

func ExampleFeatureFlags() {
	ctx := context.Background()

	engine, err := app.NewAppBuilder().
		WithContext(ctx).
		WithDynamicConfig().
		WithInitialization().
		WithRouter().
		Build()

	if err != nil {
		panic(err)
	}

	demonstrateFeatureFlags(ctx, engine)
}

func demonstrateFeatureFlags(ctx context.Context, engine *app.Engine) {
	flags := engine.GetFeatureFlags()
	if flags == nil {
		fmt.Println("feature flags not available")
		return
	}

	fmt.Println("=== Feature Flags Demo ===\n")

	fmt.Println("1. Checking boolean flags:")
	if flags.IsEnabled("enable_new_api") {
		fmt.Println("  ✓ New API is enabled")
		useNewAPI(ctx)
	} else {
		fmt.Println("  ✗ New API is disabled, using legacy API")
		useLegacyAPI(ctx)
	}

	fmt.Println("\n2. Getting string values:")
	apiVersion := flags.GetString("api_version")
	fmt.Printf("  API Version: %s\n", apiVersion)

	fmt.Println("\n3. Getting integer values:")
	maxRetries := flags.GetInt("max_retries")
	fmt.Printf("  Max Retries: %d\n", maxRetries)

	fmt.Println("\n4. Getting all flags:")
	allFlags := flags.GetAll()
	for key, value := range allFlags {
		fmt.Printf("  %s: %v\n", key, value)
	}

	fmt.Println("\n5. Dynamic flag updates:")
	flags.Set("enable_new_api", true)
	fmt.Println("  Updated enable_new_api to true")
	if flags.IsEnabled("enable_new_api") {
		fmt.Println("  ✓ New API is now enabled")
	}
}

func useNewAPI(ctx context.Context) {
	fmt.Println("  → Using new API implementation")
}

func useLegacyAPI(ctx context.Context) {
	fmt.Println("  → Using legacy API implementation")
}

