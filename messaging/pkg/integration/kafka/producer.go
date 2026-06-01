package kafka

import (
	"context"

	kafka "github.com/segmentio/kafka-go"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type producer struct {
	writer writerIface // uses the same interface defined in consumer.go
	cfg    Config
	log    logger.Service
}

// NewProducer creates a Kafka producer that writes to cfg.Topic.
// The writer uses RequireAll acks by default to guarantee durability.
// Set cfg.Async = true for fire-and-forget semantics (higher throughput, less safety).
// The underlying kafka.Writer connection is lazy; no network call is made here.
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

// Publish sends one or more messages to Kafka. When the producer was created
// with cfg.Async = false (default), Publish blocks until all brokers
// acknowledge the messages (RequireAll). Headers in each Message are forwarded
// as Kafka record headers.
//
// A single call with multiple messages is more efficient than multiple single
// calls because segmentio/kafka-go batches them in one request.
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

// Close flushes any buffered messages and closes the underlying connection.
// It must be called before the process exits to avoid losing buffered async messages.
func (p *producer) Close() error {
	return p.writer.Close()
}
