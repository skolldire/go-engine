package main

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/skolldire/go-engine/pkg/integration/aws"
)

func main() {
	ctx := context.Background()

	// 1. Crear cliente AWS (zero-config)
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	client := aws.New(cfg)

	// 2. Enviar mensaje a SQS usando helper
	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789/my-queue"

	payload := map[string]interface{}{
		"order_id": "12345",
		"status":   "created",
		"amount":   99.99,
	}

	msgID, err := aws.SQSSendMessage(ctx, client, queueURL, payload)
	if err != nil {
		log.Fatalf("failed to send message: %v", err)
	}

	log.Printf("Message sent successfully: %s", msgID)
}

