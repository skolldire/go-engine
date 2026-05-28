package kafka

import (
	"context"

	kafka "github.com/segmentio/kafka-go"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type producer struct {
	writer *kafka.Writer
	cfg    Config
	log    logger.Service
}

func NewProducer(cfg Config, log logger.Service) Producer {
	w := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
		Async:        cfg.Async,
	}

	return &producer{
		writer: w,
		cfg:    cfg,
		log:    log,
	}
}

func (p *producer) Publish(ctx context.Context, msgs ...Message) error {
	km := make([]kafka.Message, len(msgs))
	for i, m := range msgs {
		headers := make([]kafka.Header, 0, len(m.Headers))
		for k, v := range m.Headers {
			headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
		}
		km[i] = kafka.Message{
			Topic:   m.Topic,
			Key:     m.Key,
			Value:   m.Value,
			Headers: headers,
		}
	}
	return p.writer.WriteMessages(ctx, km...)
}

func (p *producer) Close() error {
	return p.writer.Close()
}
