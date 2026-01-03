//go:build example_multiple || example_all
// +build example_multiple example_all

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
		WithConfigs().
		WithInitialization().
		WithRouter().
		Build()

	if err != nil {
		panic(err)
	}

	demonstrateMultipleClients(ctx, engine)
}

func ExampleMultipleClients() {
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

	demonstrateMultipleClients(ctx, engine)
}

func demonstrateMultipleClients(ctx context.Context, engine *app.Engine) {
	fmt.Println("=== Multiple REST Clients ===")
	api1Client := engine.GetRestClient("api1")
	api2Client := engine.GetRestClient("api2")
	if api1Client != nil {
		fmt.Println("✓ API1 client available")
	}
	if api2Client != nil {
		fmt.Println("✓ API2 client available")
	}

	fmt.Println("\n=== Multiple SQS Clients ===")
	queue1Client := engine.GetSQSClientByName("queue1")
	queue2Client := engine.GetSQSClientByName("queue2")
	if queue1Client != nil {
		fmt.Println("✓ Queue1 client available")
	}
	if queue2Client != nil {
		fmt.Println("✓ Queue2 client available")
	}

	fmt.Println("\n=== Multiple SNS Clients ===")
	topic1Client := engine.GetSNSClientByName("topic1")
	topic2Client := engine.GetSNSClientByName("topic2")
	if topic1Client != nil {
		fmt.Println("✓ Topic1 client available")
	}
	if topic2Client != nil {
		fmt.Println("✓ Topic2 client available")
	}

	fmt.Println("\n=== Multiple DynamoDB Clients ===")
	db1Client := engine.GetDynamoDBClientByName("db1")
	db2Client := engine.GetDynamoDBClientByName("db2")
	if db1Client != nil {
		fmt.Println("✓ DB1 client available")
	}
	if db2Client != nil {
		fmt.Println("✓ DB2 client available")
	}

	fmt.Println("\n=== Multiple Redis Clients ===")
	cache1Client := engine.GetRedisClientByName("cache1")
	cache2Client := engine.GetRedisClientByName("cache2")
	if cache1Client != nil {
		fmt.Println("✓ Cache1 client available")
	}
	if cache2Client != nil {
		fmt.Println("✓ Cache2 client available")
	}

	fmt.Println("\n=== Multiple SQL Connections ===")
	sql1Conn := engine.GetSQLConnectionByName("db1")
	sql2Conn := engine.GetSQLConnectionByName("db2")
	if sql1Conn != nil {
		fmt.Println("✓ SQL1 connection available")
	}
	if sql2Conn != nil {
		fmt.Println("✓ SQL2 connection available")
	}

	fmt.Println("\n=== Singleton Clients (Backward Compatibility) ===")
	if engine.GetSQSClient() != nil {
		fmt.Println("✓ Singleton SQS client available")
	}
	if engine.GetRedisClient() != nil {
		fmt.Println("✓ Singleton Redis client available")
	}
	if engine.GetSQLConnection() != nil {
		fmt.Println("✓ Singleton SQL connection available")
	}
}

