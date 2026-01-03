//go:build example_sqs_sns || example_all
// +build example_sqs_sns example_all

package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
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

	demonstrateSQSSNSIntegration(ctx, engine)
}

func ExampleSQSSNSIntegration() {
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

	demonstrateSQSSNSIntegration(ctx, engine)
}

func demonstrateSQSSNSIntegration(ctx context.Context, engine *app.Engine) {
	fmt.Println("=== SQS and SNS Integration ===\n")

	queue1 := engine.GetSQSClientByName("queue1")
	if queue1 == nil {
		queue1 = engine.GetSQSClient()
	}

	topic1 := engine.GetSNSClientByName("topic1")
	if topic1 == nil {
		topic1 = engine.GetSNSClient()
	}

	if queue1 == nil || topic1 == nil {
		fmt.Println("SQS or SNS clients not configured")
		return
	}

	fmt.Println("1. SQS Operations:")
	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789/queue1"

	messageID, err := queue1.SendMsj(ctx, queueURL, "Hello from SQS", nil)
	if err != nil {
		fmt.Printf("  Send error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Message sent, ID: %s\n", messageID)
	}

	messages, err := queue1.ReceiveMsj(ctx, queueURL, 10, 20)
	if err != nil {
		fmt.Printf("  Receive error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Received %d messages\n", len(messages))
		for i, msg := range messages {
			fmt.Printf("    Message %d: %s\n", i+1, *msg.Body)
		}
	}

	fmt.Println("\n2. SNS Operations:")
	topicARN := "arn:aws:sns:us-east-1:123456789:topic1"

	messageID, err = topic1.PublishMsj(ctx, topicARN, "Hello from SNS", nil)
	if err != nil {
		fmt.Printf("  Publish error: %v\n", err)
	} else {
		fmt.Printf("  ✓ Message published, ID: %s\n", messageID)
	}

	fmt.Println("\n3. Multiple Queues and Topics:")
	queue2 := engine.GetSQSClientByName("queue2")
	if queue2 != nil {
		fmt.Println("  ✓ Queue2 client available")
		attrs := map[string]types.MessageAttributeValue{
			"priority": {
				DataType:    stringPtr("String"),
				StringValue: stringPtr("high"),
			},
		}
		_, err = queue2.SendMsj(ctx, queueURL, "High priority message", attrs)
		if err == nil {
			fmt.Println("  ✓ High priority message sent")
		}
	}

	topic2 := engine.GetSNSClientByName("topic2")
	if topic2 != nil {
		fmt.Println("  ✓ Topic2 client available")
		_, err = topic2.PublishMsj(ctx, topicARN, "Message to topic2", nil)
		if err == nil {
			fmt.Println("  ✓ Message published to topic2")
		}
	}
}

func stringPtr(s string) *string {
	return &s
}

