package rabbitmq

import (
	"context"
	"fmt"
	"net"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

func NewClient(ctx context.Context, cfg Config, log logger.Service) (Service, error) {
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
		return nil, fmt.Errorf("%w: %w", ErrConnection, err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close() // Ignore error on cleanup
		return nil, fmt.Errorf("%w: %w", ErrConnection, err)
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

	return c, nil
}

func (c *RabbitMQClient) Publish(ctx context.Context, msg Message) error {
	if msg.Body == nil {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "Publish", func(ctx context.Context) (any, error) {
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
		for {
			select {
			case <-ctx.Done():
				if c.IsLoggingEnabled() {
					c.GetLogger().Debug(ctx, "context cancelled, stopping message consumption", map[string]any{
						"queue": queue,
					})
				}
				return
			case delivery, ok := <-deliveries:
				if !ok {
					if c.IsLoggingEnabled() {
						c.GetLogger().Debug(ctx, "delivery channel closed", map[string]any{
							"queue": queue,
						})
					}
					return
				}

				// Handle Ack/Nack manually when autoAck is false
				if !autoAck {
					err := c.safeHandle(ctx, queue, delivery, handler)
					if err != nil {
						// Handler returned error, Nack the message
						if nackErr := delivery.Nack(false, true); nackErr != nil {
							if c.IsLoggingEnabled() {
								c.GetLogger().Error(ctx, nackErr, map[string]any{
									"queue":   queue,
									"message": "failed to nack message",
								})
							}
						}
						if c.IsLoggingEnabled() {
							c.GetLogger().Error(ctx, err, map[string]any{
								"queue": queue,
							})
						}
					} else {
						// Handler succeeded, Ack the message
						if ackErr := delivery.Ack(false); ackErr != nil {
							if c.IsLoggingEnabled() {
								c.GetLogger().Error(ctx, ackErr, map[string]any{
									"queue":   queue,
									"message": "failed to ack message",
								})
							}
						}
					}
				} else {
					// autoAck is true, RabbitMQ handles Ack automatically.
					// The panic containment applies here too: the broker having
					// already acked the message does not make a panicking
					// handler any less fatal to the consumer goroutine.
					if err := c.safeHandle(ctx, queue, delivery, handler); err != nil {
						if c.IsLoggingEnabled() {
							c.GetLogger().Error(ctx, err, map[string]any{
								"queue": queue,
							})
						}
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

	_, err := c.Execute(ctx, "DeclareQueue", func(ctx context.Context) (any, error) {
		return c.channel.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
	})

	return err
}

func (c *RabbitMQClient) DeclareExchange(ctx context.Context, name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	if name == "" || kind == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "DeclareExchange", func(ctx context.Context) (any, error) {
		return nil, c.channel.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
	})

	return err
}

func (c *RabbitMQClient) BindQueue(ctx context.Context, queue, routingKey, exchange string, noWait bool, args amqp.Table) error {
	if queue == "" || exchange == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "BindQueue", func(ctx context.Context) (any, error) {
		return nil, c.channel.QueueBind(queue, routingKey, exchange, noWait, args)
	})

	return err
}

func (c *RabbitMQClient) Close() error {
	if c.channel != nil {
		_ = c.channel.Close() // Ignore error, connection close will handle cleanup
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *RabbitMQClient) EnableLogging(enable bool) {
	c.SetLogging(enable)
}

// safeHandle runs the caller's handler and converts a panic into an error.
//
// The recover used to sit on the consumer goroutine, outside the delivery loop,
// so a single panicking handler unwound the whole goroutine: the consumer
// stopped reading the queue for good and the message was neither acked nor
// nacked. Containing the panic per delivery means one bad message is nacked and
// requeued while the consumer keeps running.
//
// The recover must be in the same goroutine that calls the handler; a recover
// in a parent cannot catch a panic raised in a child.
func (c *RabbitMQClient) safeHandle(
	ctx context.Context,
	queue string,
	delivery amqp.Delivery,
	handler func(amqp.Delivery) error,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrHandlerPanic, r)
			if c.IsLoggingEnabled() {
				c.GetLogger().Error(ctx, err, map[string]any{
					"queue": queue,
				})
			}
		}
	}()

	return handler(delivery)
}
