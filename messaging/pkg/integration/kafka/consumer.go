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

// readerIface abstracts kafka.Reader to enable unit testing without a real broker.
type readerIface interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// writerIface abstracts kafka.Writer to enable unit testing of both the consumer
// DLQ writer and the producer without a real broker.
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

// NewConsumer creates a Consumer that reads from cfg.Topic within cfg.GroupID.
//
// Retry behaviour: if a handler returns an error, the consumer retries up to
// cfg.MaxRetries times with linear backoff (RetryBackoff * attempt). After all
// retries are exhausted the message is routed to the DLQ (cfg.DLQTopic) or
// logged as an error if no DLQ is configured.
//
// Offset commit: the offset is committed after every message regardless of
// handler success, ensuring at-least-once delivery. The commit is synchronous
// when cfg.CommitInterval is 0 (default).
//
// Defaults applied when zero: MaxRetries = 3, RetryBackoff = 1 s.
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

// Subscribe starts a blocking message loop. It returns nil when ctx is
// cancelled (graceful shutdown). Call it in a goroutine and cancel the context
// to stop it:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	go consumer.Subscribe(ctx, handler)
//	// ...
//	cancel() // triggers graceful shutdown
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

// sendToDLQ routes a failed message to the dead-letter queue, enriching it
// with metadata headers that explain the original position and failure cause.
// When no DLQ is configured, the error is logged instead.
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

// Close shuts down the DLQ writer (if configured) and the reader.
func (c *consumer) Close() error {
	if c.dlqWriter != nil {
		_ = c.dlqWriter.Close()
	}
	return c.reader.Close()
}

// headersToMap converts a slice of kafka.Header into a string map.
// Returns nil for empty input to avoid allocating an empty map.
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
