package rabbitmq

import (
	"context"
	"fmt"
	"net"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// NewClient creates and returns a RabbitMQ client configured for the provided AMQP URL.
// It opens a network connection and channel using cfg.Timeout (or DefaultTimeout when zero) and applies logging/resilience settings from cfg; if cfg.URL is empty or establishing the connection or channel fails, it returns ErrConnection wrapped with the underlying error.
func NewClient(cfg Config, log logger.Service) (Service, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("%w: URL is required", ErrConnection)
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	// Create a dialer with the configured timeout
	dialer := &net.Dialer{
		Timeout: timeout,
	}

	// Use DialConfig to apply the timeout to the connection attempt
	conn, err := amqp.DialConfig(cfg.URL, amqp.Config{
		Dial: dialer.Dial,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrConnection, err)
	}

	baseConfig := client.BaseConfig{
		EnableLogging:  cfg.EnableLogging,
		WithResilience: cfg.WithResilience,
		Resilience:     cfg.Resilience,
		Timeout:        timeout,
	}

	c := &RabbitMQClient{
		BaseClient: client.NewBaseClientWithName(baseConfig, log, "RabbitMQ"),
		conn:       conn,
		channel:    ch,
	}

	if c.IsLoggingEnabled() {
		log.Debug(context.Background(), "RabbitMQ connection established successfully",
			map[string]interface{}{
				"url":     cfg.URL,
				"timeout": timeout.String(),
			})
	}

	return c, nil
}

func (c *RabbitMQClient) Publish(ctx context.Context, msg Message) error {
	if msg.Body == nil {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "Publish", func() (interface{}, error) {
		return nil, c.channel.PublishWithContext(ctx,
			msg.Exchange,
			msg.RoutingKey,
			msg.Mandatory,
			msg.Immediate,
			amqp.Publishing{
				Headers: msg.Headers,
				Body:    msg.Body,
			})
	})

	if err != nil {
		return c.GetLogger().WrapError(err, ErrPublishFailed.Error())
	}

	return nil
}

func (c *RabbitMQClient) Consume(ctx context.Context, queue string, autoAck bool, handler func(delivery amqp.Delivery) error) error {
	if queue == "" {
		return ErrInvalidInput
	}

	deliveries, err := c.channel.Consume(queue, "", autoAck, false, false, false, nil)
	if err != nil {
		return c.GetLogger().WrapError(err, "error consuming messages")
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				if c.IsLoggingEnabled() {
					c.GetLogger().Error(ctx, fmt.Errorf("panic in consume handler: %v", r), map[string]interface{}{
						"queue": queue,
					})
				}
			}
		}()

		for {
			select {
			case <-ctx.Done():
				if c.IsLoggingEnabled() {
					c.GetLogger().Debug(ctx, "context cancelled, stopping message consumption", map[string]interface{}{
						"queue": queue,
					})
				}
				return
			case delivery, ok := <-deliveries:
				if !ok {
					if c.IsLoggingEnabled() {
						c.GetLogger().Debug(ctx, "delivery channel closed", map[string]interface{}{
							"queue": queue,
						})
					}
					return
				}
				if err := handler(delivery); err != nil {
					if c.IsLoggingEnabled() {
						c.GetLogger().Error(ctx, err, map[string]interface{}{
							"queue": queue,
						})
					}
				}
			}
		}
	}()

	return nil
}

func (c *RabbitMQClient) DeclareQueue(ctx context.Context, name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) error {
	if name == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "DeclareQueue", func() (interface{}, error) {
		return c.channel.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
	})

	return err
}

func (c *RabbitMQClient) DeclareExchange(ctx context.Context, name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	if name == "" || kind == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "DeclareExchange", func() (interface{}, error) {
		return nil, c.channel.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
	})

	return err
}

func (c *RabbitMQClient) BindQueue(ctx context.Context, queue, routingKey, exchange string, noWait bool, args amqp.Table) error {
	if queue == "" || exchange == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "BindQueue", func() (interface{}, error) {
		return nil, c.channel.QueueBind(queue, routingKey, exchange, noWait, args)
	})

	return err
}

func (c *RabbitMQClient) Close() error {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *RabbitMQClient) EnableLogging(enable bool) {
	c.SetLogging(enable)
}