//go:build example_rest || example_all
// +build example_rest example_all

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/pkg/app"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	engine, err := app.NewAppBuilder().
		WithContext(ctx).
		WithConfigs().
		WithInitialization().
		WithRouter().
		Build()

	if err != nil {
		panic(err)
	}

	demonstrateRESTClients(ctx, engine)
}

func ExampleRESTClientUsage() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	engine, err := app.NewAppBuilder().
		WithContext(ctx).
		WithConfigs().
		WithInitialization().
		WithRouter().
		Build()

	if err != nil {
		panic(err)
	}

	demonstrateRESTClients(ctx, engine)
}

func demonstrateRESTClients(ctx context.Context, engine *app.Engine) {
	fmt.Println("=== REST Clients Usage ===\n")

	api1Client := engine.GetRestClient("api1")
	if api1Client == nil {
		fmt.Println("API1 client not configured")
		return
	}

	fmt.Println("1. GET Request:")
	headers := map[string]string{
		"Content-Type": "application/json",
		"Authorization": "Bearer token123",
	}

	resp, err := api1Client.Get(ctx, "/users", headers)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else {
		fmt.Printf("  Status: %d\n", resp.StatusCode())
		fmt.Printf("  Body: %s\n", string(resp.Body()[:min(100, len(resp.Body()))]))
	}

	fmt.Println("\n2. POST Request:")
	payload := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
	}

	resp, err = api1Client.Post(ctx, "/users", payload, headers)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else {
		fmt.Printf("  Status: %d\n", resp.StatusCode())
	}

	fmt.Println("\n3. Multiple API Clients:")
	api2Client := engine.GetRestClient("api2")
	if api2Client != nil {
		fmt.Println("  ✓ API2 client available")
		resp, err = api2Client.Get(ctx, "/products", headers)
		if err == nil {
			fmt.Printf("  Status: %d\n", resp.StatusCode())
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

