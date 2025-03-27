package sns

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
)

func NewClient(acf aws.Config, cfg Config, l logger.Service) Service {
	snsClient := sns.NewFromConfig(acf, func(o *sns.Options) {
		if cfg.BaseEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.BaseEndpoint)
		}
	})

	cliente := &Cliente{
		cliente: snsClient,
		logger:  l,
		logging: cfg.EnableLogging,
	}

	if cfg.RetryConfig != nil {
		cliente.retryer = retry_backoff.NewRetryer(retry_backoff.Dependencies{
			RetryConfig: cfg.RetryConfig,
			Logger:      l,
		})
	}

	if cfg.CircuitBreakerCfg != nil {
		cliente.circuitBreaker = circuit_breaker.NewCircuitBreaker(circuit_breaker.Dependencies{
			Config: cfg.CircuitBreakerCfg,
			Log:    l,
		})
	}

	if cliente.logging {
		endpoint := cfg.BaseEndpoint
		if endpoint == "" {
			endpoint = "AWS predeterminado"
		}
		l.Debug(context.Background(), "Cliente SNS inicializado",
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

func (c *Cliente) execute(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	if c.circuitBreaker != nil && c.retryer != nil {
		return c.executeWithCircuitBreakerAndRetry(ctx, operationName, operation)
	}

	if c.circuitBreaker != nil {
		return c.executeWithCircuitBreaker(ctx, operationName, operation)
	}

	if c.retryer != nil {
		return c.executeWithRetry(ctx, operationName, operation)
	}

	return c.executeWithLogging(ctx, operationName, operation)
}

func (c *Cliente) executeWithLogging(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	logFields := map[string]interface{}{"operacion": operationName, "servicio": "SNS"}

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

func (c *Cliente) executeWithCircuitBreakerAndRetry(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	return c.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return c.executeWithRetryInner(ctx, operationName, operation)
	})
}

func (c *Cliente) executeWithCircuitBreaker(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	return c.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return c.executeWithLogging(ctx, operationName, operation)
	})
}

func (c *Cliente) executeWithRetry(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	return c.executeWithRetryInner(ctx, operationName, operation)
}

func (c *Cliente) executeWithRetryInner(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	var result interface{}

	err := c.retryer.Do(ctx, func() error {
		res, opErr := c.executeWithLogging(ctx, operationName, operation)
		if opErr == nil {
			result = res
		}
		return opErr
	})

	return result, err
}

func (c *Cliente) CrearTema(ctx context.Context, nombre string, atributos map[string]string) (string, error) {
	if nombre == "" {
		return "", ErrInvalidInput
	}

	input := &sns.CreateTopicInput{
		Name: aws.String(nombre),
	}

	if len(atributos) > 0 {
		input.Attributes = atributos
	}

	result, err := c.execute(ctx, "CrearTema", func() (interface{}, error) {
		return c.cliente.CreateTopic(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrCrearTema.Error())
	}

	response := result.(*sns.CreateTopicOutput)
	return *response.TopicArn, nil
}

func (c *Cliente) EliminarTema(ctx context.Context, arn string) error {
	if arn == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "EliminarTema", func() (interface{}, error) {
		return c.cliente.DeleteTopic(ctx, &sns.DeleteTopicInput{
			TopicArn: aws.String(arn),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, ErrEliminarTema.Error())
	}

	return nil
}

func (c *Cliente) ListarTemas(ctx context.Context) ([]string, error) {
	result, err := c.execute(ctx, "ListarTemas", func() (interface{}, error) {
		return c.cliente.ListTopics(ctx, &sns.ListTopicsInput{})
	})

	if err != nil {
		return nil, c.logger.WrapError(err, ErrListarTemas.Error())
	}

	response := result.(*sns.ListTopicsOutput)
	temas := make([]string, len(response.Topics))

	for i, tema := range response.Topics {
		temas[i] = *tema.TopicArn
	}

	return temas, nil
}

func (c *Cliente) PublicarMensaje(ctx context.Context, temaArn string, mensaje string, atributos map[string]types.MessageAttributeValue) (string, error) {
	if temaArn == "" || mensaje == "" {
		return "", ErrInvalidInput
	}

	input := &sns.PublishInput{
		TopicArn:          aws.String(temaArn),
		Message:           aws.String(mensaje),
		MessageAttributes: atributos,
	}

	result, err := c.execute(ctx, "PublicarMensaje", func() (interface{}, error) {
		return c.cliente.Publish(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrPublicacion.Error())
	}

	response := result.(*sns.PublishOutput)
	return *response.MessageId, nil
}

func (c *Cliente) PublicarMensajeJSON(ctx context.Context, temaArn string, mensaje interface{}, atributos map[string]types.MessageAttributeValue) (string, error) {
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

	result, err := c.execute(ctx, "PublicarMensajeJSON", func() (interface{}, error) {
		return c.cliente.Publish(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrPublicacion.Error())
	}

	response := result.(*sns.PublishOutput)
	return *response.MessageId, nil
}

func (c *Cliente) CrearSuscripcion(ctx context.Context, temaArn, protocolo, endpoint string) (string, error) {
	if temaArn == "" || protocolo == "" || endpoint == "" {
		return "", ErrInvalidInput
	}

	input := &sns.SubscribeInput{
		TopicArn:              aws.String(temaArn),
		Protocol:              aws.String(protocolo),
		Endpoint:              aws.String(endpoint),
		ReturnSubscriptionArn: true,
	}

	result, err := c.execute(ctx, "CrearSuscripcion", func() (interface{}, error) {
		return c.cliente.Subscribe(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrSuscripcion.Error())
	}

	response := result.(*sns.SubscribeOutput)
	return *response.SubscriptionArn, nil
}

func (c *Cliente) EliminarSuscripcion(ctx context.Context, suscripcionArn string) error {
	if suscripcionArn == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "EliminarSuscripcion", func() (interface{}, error) {
		return c.cliente.Unsubscribe(ctx, &sns.UnsubscribeInput{
			SubscriptionArn: aws.String(suscripcionArn),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, ErrSuscripcion.Error())
	}

	return nil
}

func (c *Cliente) HabilitarLogging(activar bool) {
	c.logging = activar
}
