package main

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/skolldire/go-engine/pkg/integration/aws"
	"github.com/skolldire/go-engine/pkg/integration/observability"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/telemetry"
)

func main() {
	ctx := context.Background()

	// 1. Configurar observabilidad
	logger := logger.NewService(logger.Config{
		Level: "info",
	}, nil)

	telemetryConfig := telemetry.Config{
		ServiceName:    "my-service",
		ServiceVersion: "1.0.0",
		Environment:    "production",
		Enabled:        true,
	}
	telemetry, err := telemetry.NewTelemetry(ctx, telemetryConfig)
	if err != nil {
		log.Fatalf("failed to create telemetry: %v", err)
	}
	defer telemetry.Shutdown(ctx)

	metricsRecorder := observability.NewTelemetryMetricsRecorder(telemetry)

	// 2. Crear cliente AWS con observabilidad
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	client := aws.NewWithOptions(cfg, aws.WithObservability(
		logger,
		metricsRecorder,
		telemetry,
	))

	// 3. Usar cliente - todas las operaciones tienen logging, métricas y tracing automáticos
	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789/my-queue"
	payload := map[string]string{
		"key": "value",
	}

	msgID, err := aws.SQSSendMessage(ctx, client, queueURL, payload)
	if err != nil {
		log.Fatalf("failed to send message: %v", err)
	}

	log.Printf("Message sent: %s", msgID)
}

