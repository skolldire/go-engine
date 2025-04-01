package sqs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

func NewClient(acf aws.Config, cfg Config, l logger.Service) Service {
	sqsClient := sqs.NewFromConfig(acf, func(o *sqs.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})

	cliente := &Cliente{
		cliente: sqsClient,
		logger:  l,
		logging: cfg.EnableLogging,
	}

	if cfg.WithResilience {
		cliente.resilience = resilience.NewResilienceService(cfg.Resilience, l)
	}

	if cliente.logging {
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = "AWS predeterminado"
		}
		l.Debug(context.Background(), "Cliente SQS inicializado",
			map[string]interface{}{
				"endpoint": endpoint,
			})
	}

	return cliente
}

func (c *Cliente) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return context.WithTimeout(ctx, DefaultTimeout)
	}
	return context.WithCancel(ctx)
}

func (c *Cliente) execute(ctx context.Context, operationName string,
	operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	return c.executeOperation(ctx, operationName, operation)
}

func (c *Cliente) executeOperation(ctx context.Context, operationName string,
	operation func() (interface{}, error)) (interface{}, error) {
	logFields := map[string]interface{}{"operacion": operationName, "servicio": "SQS"}

	if c.resilience != nil {
		return c.executeWithResilience(ctx, operationName, operation, logFields)
	}

	return c.executeWithLogging(ctx, operationName, operation, logFields)
}

func (c *Cliente) executeWithResilience(ctx context.Context, operationName string,
	operation func() (interface{}, error), logFields map[string]interface{}) (interface{}, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Iniciando operación SQS con resiliencia: %s", operationName), logFields)
	}

	result, err := c.resilience.Execute(ctx, operation)

	if err != nil && c.logging {
		c.logger.Error(ctx, err, logFields)
	} else if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Operación SQS completada con resiliencia: %s", operationName), logFields)
	}

	return result, err
}

func (c *Cliente) executeWithLogging(ctx context.Context, operationName string,
	operation func() (interface{}, error), logFields map[string]interface{}) (interface{}, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Iniciando operación SQS: %s", operationName), logFields)
	}

	result, err := operation()

	if err != nil && c.logging {
		c.logger.Error(ctx, err, logFields)
	} else if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Operación SQS completada: %s", operationName), logFields)
	}

	return result, err
}

func (c *Cliente) SendMsj(ctx context.Context, queueURL string, mensaje string,
	atributos map[string]types.MessageAttributeValue) (string, error) {
	if queueURL == "" || mensaje == "" {
		return "", ErrInvalidInput
	}

	input := &sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(mensaje),
		MessageAttributes: atributos,
	}

	result, err := c.execute(ctx, "SendMsj", func() (interface{}, error) {
		return c.cliente.SendMessage(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrEnviarMensaje.Error())
	}

	response := result.(*sqs.SendMessageOutput)
	return *response.MessageId, nil
}

func (c *Cliente) SendJSON(ctx context.Context, queueURL string, mensaje interface{},
	atributos map[string]types.MessageAttributeValue) (string, error) {
	if queueURL == "" || mensaje == nil {
		return "", ErrInvalidInput
	}

	jsonBytes, err := json.Marshal(mensaje)
	if err != nil {
		return "", fmt.Errorf("error al convertir mensaje a JSON: %w", err)
	}

	return c.SendMsj(ctx, queueURL, string(jsonBytes), atributos)
}

func (c *Cliente) ReceiveMsj(ctx context.Context, queueURL string, maxMensajes int32,
	tiempoEspera int32) ([]types.Message, error) {
	if queueURL == "" {
		return nil, ErrInvalidInput
	}

	if maxMensajes <= 0 {
		maxMensajes = 10 // Valor por defecto
	}

	input := &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(queueURL),
		MaxNumberOfMessages:   maxMensajes,
		WaitTimeSeconds:       tiempoEspera,
		MessageAttributeNames: []string{"All"},
	}

	result, err := c.execute(ctx, "RecibirMensajes", func() (interface{}, error) {
		return c.cliente.ReceiveMessage(ctx, input)
	})

	if err != nil {
		return nil, c.logger.WrapError(err, ErrRecibirMensajes.Error())
	}

	response := result.(*sqs.ReceiveMessageOutput)
	return response.Messages, nil
}

func (c *Cliente) DeleteMsj(ctx context.Context, queueURL string, receiptHandle string) error {
	if queueURL == "" || receiptHandle == "" {
		return ErrInvalidInput
	}

	input := &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	}

	_, err := c.execute(ctx, "DeleteMsj", func() (interface{}, error) {
		return c.cliente.DeleteMessage(ctx, input)
	})

	if err != nil {
		return c.logger.WrapError(err, ErrEliminarMensaje.Error())
	}

	return nil
}

func (c *Cliente) CreateQueue(ctx context.Context, nombre string, atributos map[string]string) (string, error) {
	if nombre == "" {
		return "", ErrInvalidInput
	}

	input := &sqs.CreateQueueInput{
		QueueName: aws.String(nombre),
	}

	if len(atributos) > 0 {
		input.Attributes = atributos
	}

	result, err := c.execute(ctx, "CreateQueue", func() (interface{}, error) {
		return c.cliente.CreateQueue(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrCrearCola.Error())
	}

	response := result.(*sqs.CreateQueueOutput)
	return *response.QueueUrl, nil
}

func (c *Cliente) DeleteQueue(ctx context.Context, queueURL string) error {
	if queueURL == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "DeleteQueue", func() (interface{}, error) {
		return c.cliente.DeleteQueue(ctx, &sqs.DeleteQueueInput{
			QueueUrl: aws.String(queueURL),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, ErrEliminarCola.Error())
	}

	return nil
}

func (c *Cliente) ListQueue(ctx context.Context, prefijo string) ([]string, error) {
	input := &sqs.ListQueuesInput{}
	if prefijo != "" {
		input.QueueNamePrefix = aws.String(prefijo)
	}

	result, err := c.execute(ctx, "ListQueue", func() (interface{}, error) {
		return c.cliente.ListQueues(ctx, input)
	})

	if err != nil {
		return nil, c.logger.WrapError(err, ErrListarColas.Error())
	}

	response := result.(*sqs.ListQueuesOutput)
	urls := make([]string, len(response.QueueUrls))
	copy(urls, response.QueueUrls)

	return urls, nil
}

func (c *Cliente) GetURLQueue(ctx context.Context, nombre string) (string, error) {
	if nombre == "" {
		return "", ErrInvalidInput
	}

	result, err := c.execute(ctx, "GetURLQueue", func() (interface{}, error) {
		return c.cliente.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
			QueueName: aws.String(nombre),
		})
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrObtenerURLCola.Error())
	}

	response := result.(*sqs.GetQueueUrlOutput)
	return *response.QueueUrl, nil
}

func (c *Cliente) EnableLogging(activar bool) {
	c.logging = activar
}
