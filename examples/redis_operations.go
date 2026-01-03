//go:build example_redis || example_all
// +build example_redis example_all

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/pkg/app"
)

func main() {
	ctx := context.Background()

	engine, err := app.NewAppBuilder().
		WithContext(ctx).
		WithConfigs().
		WithInitialization().
		WithRouter().
		Build()

	if err != nil {
		panic(err)
	}

	demonstrateRedisOperations(ctx, engine)
}

func ExampleRedisOperations() {
	ctx := context.Background()

	engine, err := app.NewAppBuilder().
		WithContext(ctx).
		WithConfigs().
		WithInitialization().
		WithRouter().
		Build()

	if err != nil {
		panic(err)
	}

	demonstrateRedisOperations(ctx, engine)
}

func demonstrateRedisOperations(ctx context.Context, engine *app.Engine) {
	fmt.Println("=== Redis Operations ===\n")

	cache1 := engine.GetRedisClientByName("cache1")
	if cache1 == nil {
		cache1 = engine.GetRedisClient()
	}

	if cache1 == nil {
		fmt.Println("Redis client not configured")
		return
	}

	fmt.Println("1. Basic Operations:")
	err := cache1.Set(ctx, "user:1", "John Doe", 10*time.Minute)
	if err != nil {
		fmt.Printf("  Set error: %v\n", err)
	} else {
		fmt.Println("  ✓ Set operation successful")
	}

	value, err := cache1.Get(ctx, "user:1")
	if err != nil {
		fmt.Printf("  Get error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Get value: %s\n", value)
	}

	fmt.Println("\n2. Hash Operations:")
	hsetCount, err := cache1.HSet(ctx, "user:profile:1", "name", "John", "age", "30", "city", "NYC")
	if err != nil {
		fmt.Printf("  HSet error: %v\n", err)
	} else {
		fmt.Printf("  ✓ HSet operation successful, set %d fields\n", hsetCount)
	}

	profile, err := cache1.HGetAll(ctx, "user:profile:1")
	if err != nil {
		fmt.Printf("  HGetAll error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Profile: %v\n", profile)
	}

	fmt.Println("\n3. List Operations:")
	lpushCount, err := cache1.LPush(ctx, "tasks", "task1", "task2", "task3")
	if err != nil {
		fmt.Printf("  LPush error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Pushed %d items\n", lpushCount)
	}

	task, err := cache1.RPop(ctx, "tasks")
	if err != nil {
		fmt.Printf("  RPop error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Popped task: %s\n", task)
	}

	fmt.Println("\n4. Set Operations:")
	saddCount, err := cache1.SAdd(ctx, "tags", "golang", "redis", "cache")
	if err != nil {
		fmt.Printf("  SAdd error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Added %d tags\n", saddCount)
	}

	members, err := cache1.SMembers(ctx, "tags")
	if err != nil {
		fmt.Printf("  SMembers error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Tags: %v\n", members)
	}

	fmt.Println("\n5. Multiple Redis Clients:")
	cache2 := engine.GetRedisClientByName("cache2")
	if cache2 != nil {
		fmt.Println("  ✓ Cache2 client available")
		err = cache2.Set(ctx, "cache2:key", "value", 5*time.Minute)
		if err == nil {
			fmt.Println("  ✓ Cache2 operation successful")
		}
	}
}

