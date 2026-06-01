package kafka

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type client struct {
	prod Producer
	cons Consumer
	cfg  Config
}

// NewClient creates a composite Kafka client that exposes both Producer and
// Consumer under a single handle. It also implements health.Checker via Ping.
//
// The underlying kafka.Writer and kafka.Reader connect lazily on first use;
// NewClient itself makes no network calls.
func NewClient(cfg Config, log logger.Service) (Client, error) {
	return &client{
		prod: NewProducer(cfg, log),
		cons: NewConsumer(cfg, log),
		cfg:  cfg,
	}, nil
}

// Publish delegates to the embedded Producer.
func (c *client) Publish(ctx context.Context, msgs ...Message) error {
	return c.prod.Publish(ctx, msgs...)
}

// Subscribe delegates to the embedded Consumer.
func (c *client) Subscribe(ctx context.Context, handler Handler) error {
	return c.cons.Subscribe(ctx, handler)
}

// Close shuts down both the producer and consumer. The producer error (if any)
// is returned; the consumer close error is silently discarded only if the
// producer already failed.
func (c *client) Close() error {
	prodErr := c.prod.Close()
	consErr := c.cons.Close()
	if prodErr != nil {
		return prodErr
	}
	return consErr
}

// Ping performs a TCP dial to each configured broker and returns nil as soon
// as one responds. Used by the health check subsystem (RegisterHealthChecker).
func (c *client) Ping(ctx context.Context) error {
	return NewChecker(c.cfg.Brokers).Check(ctx)
}
