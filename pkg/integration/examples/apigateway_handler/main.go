package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/skolldire/go-engine/pkg/integration/aws"
	"github.com/skolldire/go-engine/pkg/integration/inbound"
)

// RequestPayload representa el body del request
type RequestPayload struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Handler procesa requests de API Gateway usando la capa de integración
func Handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// 1. Normalizar evento API Gateway a Request
	req, err := inbound.NormalizeAPIGatewayEvent(&event)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       `{"error":"failed to normalize event"}`,
		}, nil
	}

	// 2. Parsear body desde bytes
	var payload RequestPayload
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       `{"error":"invalid request body"}`,
		}, nil
	}

	// 3. Procesar request
	log.Printf("Processing request: name=%s, email=%s", payload.Name, payload.Email)

	// 4. Crear cliente AWS si necesitas invocar otros servicios
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       `{"error":"failed to load AWS config"}`,
		}, nil
	}
	client := aws.New(cfg)

	// Ejemplo: Enviar notificación a SNS
	topicARN := "arn:aws:sns:us-east-1:123456789:notifications"
	notification := map[string]interface{}{
		"type":  "user_registered",
		"name":  payload.Name,
		"email": payload.Email,
	}

	_, err = aws.SNSPublish(ctx, client, topicARN, notification)
	if err != nil {
		log.Printf("failed to publish notification: %v", err)
		// Continuar aunque falle la notificación
	}

	// 5. Retornar respuesta
	response := map[string]interface{}{
		"message": "User registered successfully",
		"name":    payload.Name,
	}

	responseBody, _ := json.Marshal(response)
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(responseBody),
	}, nil
}

func main() {
	lambda.Start(Handler)
}

