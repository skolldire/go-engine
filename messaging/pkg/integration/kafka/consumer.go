package kafka

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	kafka "github.com/segmentio/kafka-go"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

const (
	defaultMaxRetries   = 3
	defaultRetryBackoff = time.Second
)

type readerIface interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type writerIface interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type consumer struct {
	reader    readerIface
	dlqWriter writerIface
	cfg       Config
	log       logger.Service
}

func NewConsumer(cfg Config, log logger.Service) Consumer {
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = defaultRetryBackoff
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       cfg.MinBytes,
		MaxBytes:       cfg.MaxBytes,
		MaxWait:        cfg.MaxWait,
		CommitInterval: cfg.CommitInterval,
	})

	var dlq writerIface
	if cfg.DLQTopic != "" {
		dlq = &kafka.Writer{
			Addr:         kafka.TCP(cfg.Brokers...),
			Topic:        cfg.DLQTopic,
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireAll,
		}
	}

	return &consumer{
		reader:    r,
		dlqWriter: dlq,
		cfg:       cfg,
		log:       log,
	}
}

func (c *consumer) Subscribe(ctx context.Context, handler Handler) error {
	for {
		km, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return err
		}

		msg := Message{
			Topic:     km.Topic,
			Key:       km.Key,
			Value:     km.Value,
			Offset:    km.Offset,
			Partition: km.Partition,
			Time:      km.Time,
			Headers:   headersToMap(km.Headers),
		}

		var handlerErr error
		for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
			if attempt > 0 {
				time.Sleep(c.cfg.RetryBackoff * time.Duration(attempt))
			}
			if handlerErr = handler(ctx, msg); handlerErr == nil {
				break
			}
		}

		if handlerErr != nil {
			c.sendToDLQ(ctx, km, handlerErr)
		}

		if commitErr := c.reader.CommitMessages(ctx, km); commitErr != nil {
			c.log.Warn(ctx, "failed to commit kafka message", map[string]interface{}{
				"offset": km.Offset,
				"error":  commitErr.Error(),
			})
		}
	}
}

func (c *consumer) sendToDLQ(ctx context.Context, km kafka.Message, cause error) {
	if c.dlqWriter == nil {
		c.log.Error(ctx, cause, map[string]interface{}{
			"topic":  km.Topic,
			"offset": km.Offset,
			"cause":  cause.Error(),
		})
		return
	}

	dlqMsg := kafka.Message{
		Value: km.Value,
		Key:   km.Key,
		Headers: append(km.Headers,
			kafka.Header{Key: "x-original-topic", Value: []byte(km.Topic)},
			kafka.Header{Key: "x-original-offset", Value: []byte(strconv.FormatInt(km.Offset, 10))},
			kafka.Header{Key: "x-error", Value: []byte(cause.Error())},
			kafka.Header{Key: "x-failed-at", Value: []byte(time.Now().UTC().Format(time.RFC3339))},
		),
	}

	if err := c.dlqWriter.WriteMessages(ctx, dlqMsg); err != nil {
		c.log.Error(ctx, fmt.Errorf("dlq write failed: %w", err), map[string]interface{}{
			"original_topic":  km.Topic,
			"original_offset": km.Offset,
		})
	}
}

func (c *consumer) Close() error {
	if c.dlqWriter != nil {
		_ = c.dlqWriter.Close()
	}
	return c.reader.Close()
}

func headersToMap(headers []kafka.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	m := make(map[string]string, len(headers))
	for _, h := range headers {
		m[h.Key] = string(h.Value)
	}
	return m
}
