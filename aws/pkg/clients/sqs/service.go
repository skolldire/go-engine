package sqs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

func NewClient(acf aws.Config, cfg Config, l logger.Service) Service {
	sqsClient := sqs.NewFromConfig(acf, func(o *sqs.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})

	return &Cliente{
		cliente: sqsClient,
		BaseClient: client.NewBaseClientWithName(client.BaseConfig{
			EnableLogging:  cfg.EnableLogging,
			WithResilience: cfg.WithResilience,
			Resilience:     cfg.Resilience,
			Timeout:        DefaultTimeout,
		}, l, "SQS"),
	}
}

func (c *Cliente) SendMessage(ctx context.Context, queueURL string, mensaje string,
	atributos map[string]types.MessageAttributeValue) (string, error) {
	if queueURL == "" || mensaje == "" {
		return "", ErrInvalidInput
	}

	input := &sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(mensaje),
		MessageAttributes: atributos,
	}

	result, err := c.Execute(ctx, "SendMessage", func(ctx context.Context) (any, error) {
		return c.cliente.SendMessage(ctx, input)
	})

	if err != nil {
		return "", c.GetLogger().WrapError(err, ErrSendMessage.Error())
	}

	response, err := client.SafeTypeAssert[*sqs.SendMessageOutput](result)
	if err != nil {
		return "", c.GetLogger().WrapError(err, ErrSendMessage.Error())
	}
	if response == nil || response.MessageId == nil {
		return "", c.GetLogger().WrapError(ErrSendMessage, "SQS response or MessageId is nil")
	}
	return *response.MessageId, nil
}

func (c *Cliente) SendJSON(ctx context.Context, queueURL string, mensaje any,
	atributos map[string]types.MessageAttributeValue) (string, error) {
	if queueURL == "" || mensaje == nil {
		return "", ErrInvalidInput
	}

	jsonBytes, err := json.Marshal(mensaje)
	if err != nil {
		return "", fmt.Errorf("error converting message to JSON: %w", err)
	}

	return c.SendMessage(ctx, queueURL, string(jsonBytes), atributos)
}

func (c *Cliente) ReceiveMessages(ctx context.Context, queueURL string, maxMensajes int32,
	tiempoEspera int32) ([]types.Message, error) {
	if queueURL == "" {
		return nil, ErrInvalidInput
	}

	if maxMensajes <= 0 {
		maxMensajes = 10
	}

	input := &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(queueURL),
		MaxNumberOfMessages:   maxMensajes,
		WaitTimeSeconds:       tiempoEspera,
		MessageAttributeNames: []string{"All"},
	}

	result, err := c.Execute(ctx, "RecibirMensajes", func(ctx context.Context) (any, error) {
		return c.cliente.ReceiveMessage(ctx, input)
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrReceiveMessages.Error())
	}

	response, err := client.SafeTypeAssert[*sqs.ReceiveMessageOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrReceiveMessages.Error())
	}
	if response == nil {
		return nil, c.GetLogger().WrapError(fmt.Errorf("received nil response"), ErrReceiveMessages.Error())
	}
	return response.Messages, nil
}

func (c *Cliente) DeleteMessage(ctx context.Context, queueURL string, receiptHandle string) error {
	if queueURL == "" || receiptHandle == "" {
		return ErrInvalidInput
	}

	input := &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	}

	_, err := c.Execute(ctx, "DeleteMessage", func(ctx context.Context) (any, error) {
		return c.cliente.DeleteMessage(ctx, input)
	})

	if err != nil {
		return c.GetLogger().WrapError(err, ErrDeleteMessage.Error())
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

	result, err := c.Execute(ctx, "CreateQueue", func(ctx context.Context) (any, error) {
		return c.cliente.CreateQueue(ctx, input)
	})

	if err != nil {
		return "", c.GetLogger().WrapError(err, ErrCreateQueue.Error())
	}

	response, err := client.SafeTypeAssert[*sqs.CreateQueueOutput](result)
	if err != nil {
		return "", c.GetLogger().WrapError(err, ErrCreateQueue.Error())
	}
	if response == nil || response.QueueUrl == nil {
		return "", c.GetLogger().WrapError(ErrCreateQueue, "SQS response or QueueUrl is nil")
	}
	return *response.QueueUrl, nil
}

func (c *Cliente) DeleteQueue(ctx context.Context, queueURL string) error {
	if queueURL == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "DeleteQueue", func(ctx context.Context) (any, error) {
		return c.cliente.DeleteQueue(ctx, &sqs.DeleteQueueInput{
			QueueUrl: aws.String(queueURL),
		})
	})

	if err != nil {
		return c.GetLogger().WrapError(err, ErrDeleteQueue.Error())
	}

	return nil
}

func (c *Cliente) ListQueue(ctx context.Context, prefijo string) ([]string, error) {
	input := &sqs.ListQueuesInput{}
	if prefijo != "" {
		input.QueueNamePrefix = aws.String(prefijo)
	}

	result, err := c.Execute(ctx, "ListQueue", func(ctx context.Context) (any, error) {
		return c.cliente.ListQueues(ctx, input)
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrListQueues.Error())
	}

	response, err := client.SafeTypeAssert[*sqs.ListQueuesOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrListQueues.Error())
	}
	urls := make([]string, len(response.QueueUrls))
	copy(urls, response.QueueUrls)

	return urls, nil
}

func (c *Cliente) GetURLQueue(ctx context.Context, nombre string) (string, error) {
	if nombre == "" {
		return "", ErrInvalidInput
	}

	result, err := c.Execute(ctx, "GetURLQueue", func(ctx context.Context) (any, error) {
		return c.cliente.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
			QueueName: aws.String(nombre),
		})
	})

	if err != nil {
		return "", c.GetLogger().WrapError(err, ErrGetQueueURL.Error())
	}

	response, err := client.SafeTypeAssert[*sqs.GetQueueUrlOutput](result)
	if err != nil {
		return "", c.GetLogger().WrapError(err, ErrGetQueueURL.Error())
	}
	if response == nil || response.QueueUrl == nil {
		return "", c.GetLogger().WrapError(ErrGetQueueURL, "SQS response or QueueUrl is nil")
	}
	return *response.QueueUrl, nil
}

func (c *Cliente) EnableLogging(activar bool) {
	c.SetLogging(activar)
}
