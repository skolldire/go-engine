package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/skolldire/go-engine/pkg/integration/aws"
	"github.com/skolldire/go-engine/pkg/integration/inbound"
	_ "github.com/aws/aws-lambda-go/lambda" // Para referencia de uso
)

// OrderPayload representa el payload del mensaje
type OrderPayload struct {
	OrderID string  `json:"order_id"`
	Status  string  `json:"status"`
	Amount  float64 `json:"amount"`
}

// Handler procesa eventos SQS usando la capa de integración
func Handler(ctx context.Context, event events.SQSEvent) error {
	// 1. Normalizar evento SQS a Requests
	requests, err := inbound.NormalizeSQSEvent(&event)
	if err != nil {
		return err
	}

	// 2. Crear cliente AWS
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	client := aws.New(cfg)

	// 3. Procesar cada mensaje
	for _, req := range requests {
		// Parsear body desde bytes
		var order OrderPayload
		if err := json.Unmarshal(req.Body, &order); err != nil {
			log.Printf("failed to unmarshal order: %v", err)
			continue
		}

		// Procesar orden
		log.Printf("Processing order: %s, status: %s, amount: %.2f",
			order.OrderID, order.Status, order.Amount)

		// Ejemplo: Invocar otra Lambda para procesar el pedido
		result := map[string]interface{}{
			"order_id": order.OrderID,
			"processed": true,
		}

		_, err := aws.LambdaInvoke(ctx, client, "process-order-function", result)
		if err != nil {
			log.Printf("failed to invoke Lambda: %v", err)
			continue
		}

		log.Printf("Order %s processed successfully", order.OrderID)
	}

	return nil
}

// Para usar este handler en Lambda, usa:
// func main() { lambda.Start(Handler) }
//
// Para testing local, puedes usar esta función main:
func main() {
	// Este es un ejemplo de handler Lambda
	// Para ejecutarlo localmente, necesitarías crear un evento SQS de prueba
	log.Println("Este es un handler Lambda. Para usarlo en Lambda, descomenta:")
	log.Println("func main() { lambda.Start(Handler) }")
}

