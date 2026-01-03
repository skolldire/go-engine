//go:build example_dynamodb || example_all
// +build example_dynamodb example_all

package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/skolldire/go-engine/pkg/app"
)

type User struct {
	ID    string `dynamodbav:"id"`
	Name  string `dynamodbav:"name"`
	Email string `dynamodbav:"email"`
}

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

	demonstrateDynamoDBOperations(ctx, engine)
}

func ExampleDynamoDBOperations() {
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

	demonstrateDynamoDBOperations(ctx, engine)
}

func demonstrateDynamoDBOperations(ctx context.Context, engine *app.Engine) {
	fmt.Println("=== DynamoDB Operations ===\n")

	db1 := engine.GetDynamoDBClientByName("db1")
	if db1 == nil {
		db1 = engine.GetDynamoDBClient()
	}

	if db1 == nil {
		fmt.Println("DynamoDB client not configured")
		return
	}

	tableName := "users"

	fmt.Println("1. Put Item:")
	user := User{
		ID:    "user-123",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	item, err := attributevalue.MarshalMap(user)
	if err != nil {
		fmt.Printf("  Marshal error: %v\n", err)
		return
	}

	putInput := &dynamodb.PutItemInput{
		TableName: &tableName,
		Item:      item,
	}

	_, err = db1.PutItem(ctx, putInput)
	if err != nil {
		fmt.Printf("  PutItem error: %v\n", err)
	} else {
		fmt.Println("  ✓ User saved successfully")
	}

	fmt.Println("\n2. Get Item:")
	key := map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "user-123"},
	}

	getInput := &dynamodb.GetItemInput{
		TableName: &tableName,
		Key:       key,
	}

	result, err := db1.GetItem(ctx, getInput)
	if err != nil {
		fmt.Printf("  GetItem error: %v\n", err)
	} else if result.Item != nil {
		var retrievedUser User
		err = attributevalue.UnmarshalMap(result.Item, &retrievedUser)
		if err != nil {
			fmt.Printf("  Unmarshal error: %v\n", err)
		} else {
			fmt.Printf("  ✓ Retrieved user: %+v\n", retrievedUser)
		}
	} else {
		fmt.Println("  Item not found")
	}

	fmt.Println("\n3. Query Operations:")
	indexName := "email-index"
	keyCondition := "email = :email"
	expressionValues := map[string]types.AttributeValue{
		":email": &types.AttributeValueMemberS{Value: "john@example.com"},
	}

	queryInput := &dynamodb.QueryInput{
		TableName:              &tableName,
		IndexName:              &indexName,
		KeyConditionExpression: &keyCondition,
		ExpressionAttributeValues: expressionValues,
	}

	queryResult, err := db1.Query(ctx, queryInput)
	if err != nil {
		fmt.Printf("  Query error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Query returned %d items\n", len(queryResult.Items))
	}

	fmt.Println("\n4. Multiple DynamoDB Clients:")
	db2 := engine.GetDynamoDBClientByName("db2")
	if db2 != nil {
		fmt.Println("  ✓ DB2 client available")
	}
}

