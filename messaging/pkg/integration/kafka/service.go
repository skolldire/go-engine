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

func NewClient(cfg Config, log logger.Service) (Client, error) {
	return &client{
		prod: NewProducer(cfg, log),
		cons: NewConsumer(cfg, log),
		cfg:  cfg,
	}, nil
}

func (c *client) Publish(ctx context.Context, msgs ...Message) error {
	return c.prod.Publish(ctx, msgs...)
}

func (c *client) Subscribe(ctx context.Context, handler Handler) error {
	return c.cons.Subscribe(ctx, handler)
}

func (c *client) Close() error {
	prodErr := c.prod.Close()
	consErr := c.cons.Close()
	if prodErr != nil {
		return prodErr
	}
	return consErr
}

func (c *client) Ping(ctx context.Context) error {
	return NewChecker(c.cfg.Brokers).Check(ctx)
}
