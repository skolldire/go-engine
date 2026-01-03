package rabbitmq

import (
	"context"
	"errors"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout = 5 * time.Second
)

var (
	ErrConnection   = errors.New("rabbitmq connection error")
	ErrInvalidInput = errors.New("invalid input")
	ErrPublishFailed = errors.New("error publishing message")
)

type Config struct {
	URL            string            `mapstructure:"url" json:"url"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
	Timeout        time.Duration     `mapstructure:"timeout" json:"timeout"`
}

type Message struct {
	Exchange   string
	RoutingKey string
	Body       []byte
	Headers    map[string]interface{}
	Mandatory  bool
	Immediate  bool
}

type Service interface {
	// Publish publishes a message to an exchange.
	Publish(ctx context.Context, msg Message) error
	
	// Consume starts consuming messages from a queue.
	// The handler function is called for each message.
	// The goroutine respects context cancellation and will stop when context is cancelled.
	Consume(ctx context.Context, queue string, autoAck bool, handler func(delivery amqp.Delivery) error) error
	
	// DeclareQueue declares a queue with the given properties.
	DeclareQueue(ctx context.Context, name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) error
	
	// DeclareExchange declares an exchange with the given properties.
	DeclareExchange(ctx context.Context, name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	
	// BindQueue binds a queue to an exchange with a routing key.
	BindQueue(ctx context.Context, queue, routingKey, exchange string, noWait bool, args amqp.Table) error
	
	// Close closes the connection and channel.
	// Should be called when done using the client.
	Close() error
	
	// EnableLogging enables or disables logging for this client.
	EnableLogging(enable bool)
}

type RabbitMQClient struct {
	*client.BaseClient
	conn    *amqp.Connection
	channel *amqp.Channel
}

