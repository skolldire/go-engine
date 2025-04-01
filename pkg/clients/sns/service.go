package sns

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

func NewClient(acf aws.Config, cfg Config, log logger.Service) Service {
	snsClient := sns.NewFromConfig(acf, func(o *sns.Options) {
		if cfg.BaseEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.BaseEndpoint)
		}
	})

	cliente := &Cliente{
		cliente: snsClient,
		logger:  log,
		logging: cfg.EnableLogging,
	}

	if cfg.WithResilience {
		cliente.resilience = resilience.NewResilienceService(cfg.Resilience, log)
	}

	if cliente.logging {
		endpoint := cfg.BaseEndpoint
		if endpoint == "" {
			endpoint = "AWS predeterminado"
		}
		log.Debug(context.Background(), "Cliente SNS inicializado",
			map[string]interface{}{
				"endpoint": endpoint,
			})
	}

	return cliente
}

func (c *Cliente) execute(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	return c.executeOperation(ctx, operationName, operation)
}

func (c *Cliente) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return context.WithTimeout(ctx, DefaultTimeout)
	}
	return context.WithCancel(ctx)
}

func (c *Cliente) executeOperation(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	logFields := map[string]interface{}{"operacion": operationName, "servicio": "SNS"}

	if c.resilience != nil {
		return c.executeWithResilience(ctx, operationName, operation, logFields)
	}

	return c.executeWithLogging(ctx, operationName, operation, logFields)
}

func (c *Cliente) executeWithResilience(ctx context.Context, operationName string, operation func() (interface{}, error), logFields map[string]interface{}) (interface{}, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Iniciando operación SNS con resiliencia: %s", operationName), logFields)
	}

	result, err := c.resilience.Execute(ctx, operation)

	if err != nil && c.logging {
		c.logger.Error(ctx, err, logFields)
	} else if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Operación SNS completada con resiliencia: %s", operationName), logFields)
	}

	return result, err
}

func (c *Cliente) executeWithLogging(ctx context.Context, operationName string, operation func() (interface{}, error), logFields map[string]interface{}) (interface{}, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Iniciando operación SNS: %s", operationName), logFields)
	}

	result, err := operation()

	if err != nil && c.logging {
		c.logger.Error(ctx, err, logFields)
	} else if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Operación SNS completada: %s", operationName), logFields)
	}

	return result, err
}

func (c *Cliente) CreateTopic(ctx context.Context, nombre string, atributos map[string]string) (string, error) {
	if nombre == "" {
		return "", ErrInvalidInput
	}

	input := &sns.CreateTopicInput{
		Name: aws.String(nombre),
	}

	if len(atributos) > 0 {
		input.Attributes = atributos
	}

	result, err := c.execute(ctx, "CreateTopic", func() (interface{}, error) {
		return c.cliente.CreateTopic(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrCreateTopic.Error())
	}

	response := result.(*sns.CreateTopicOutput)
	return *response.TopicArn, nil
}

func (c *Cliente) DeleteTopic(ctx context.Context, arn string) error {
	if arn == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "DeleteTopic", func() (interface{}, error) {
		return c.cliente.DeleteTopic(ctx, &sns.DeleteTopicInput{
			TopicArn: aws.String(arn),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, ErrDeleteTopic.Error())
	}

	return nil
}

func (c *Cliente) GetTopics(ctx context.Context) ([]string, error) {
	result, err := c.execute(ctx, "GetTopics", func() (interface{}, error) {
		return c.cliente.ListTopics(ctx, &sns.ListTopicsInput{})
	})

	if err != nil {
		return nil, c.logger.WrapError(err, ErrListTopics.Error())
	}

	response := result.(*sns.ListTopicsOutput)
	temas := make([]string, len(response.Topics))

	for i, tema := range response.Topics {
		temas[i] = *tema.TopicArn
	}

	return temas, nil
}

func (c *Cliente) PublishMsj(ctx context.Context, temaArn string, mensaje string, atributos map[string]types.MessageAttributeValue) (string, error) {
	if temaArn == "" || mensaje == "" {
		return "", ErrInvalidInput
	}

	input := &sns.PublishInput{
		TopicArn:          aws.String(temaArn),
		Message:           aws.String(mensaje),
		MessageAttributes: atributos,
	}

	result, err := c.execute(ctx, "PublishMsj", func() (interface{}, error) {
		return c.cliente.Publish(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrPublication.Error())
	}

	response := result.(*sns.PublishOutput)
	return *response.MessageId, nil
}

func (c *Cliente) PublishJSON(ctx context.Context, temaArn string, mensaje interface{}, atributos map[string]types.MessageAttributeValue) (string, error) {
	if temaArn == "" || mensaje == nil {
		return "", ErrInvalidInput
	}

	jsonBytes, err := json.Marshal(mensaje)
	if err != nil {
		return "", fmt.Errorf("error al convertir mensaje a JSON: %w", err)
	}

	input := &sns.PublishInput{
		TopicArn:          aws.String(temaArn),
		Message:           aws.String(string(jsonBytes)),
		MessageStructure:  aws.String("json"),
		MessageAttributes: atributos,
	}

	result, err := c.execute(ctx, "PublishJSON", func() (interface{}, error) {
		return c.cliente.Publish(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrPublication.Error())
	}

	response := result.(*sns.PublishOutput)
	return *response.MessageId, nil
}

func (c *Cliente) CreateSubscription(ctx context.Context, temaArn, protocolo, endpoint string) (string, error) {
	if temaArn == "" || protocolo == "" || endpoint == "" {
		return "", ErrInvalidInput
	}

	input := &sns.SubscribeInput{
		TopicArn:              aws.String(temaArn),
		Protocol:              aws.String(protocolo),
		Endpoint:              aws.String(endpoint),
		ReturnSubscriptionArn: true,
	}

	result, err := c.execute(ctx, "CreateSubscription", func() (interface{}, error) {
		return c.cliente.Subscribe(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrSubscription.Error())
	}

	response := result.(*sns.SubscribeOutput)
	return *response.SubscriptionArn, nil
}

func (c *Cliente) DeleteSubscription(ctx context.Context, suscripcionArn string) error {
	if suscripcionArn == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "DeleteSubscription", func() (interface{}, error) {
		return c.cliente.Unsubscribe(ctx, &sns.UnsubscribeInput{
			SubscriptionArn: aws.String(suscripcionArn),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, ErrSubscription.Error())
	}

	return nil
}

func (c *Cliente) EnableLogging(activar bool) {
	c.logging = activar
}
