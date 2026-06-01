package kafka

import (
	"context"
	"time"
)

// Config holds the configuration for a Kafka client (producer and consumer).
// It is populated from the application's YAML section `kafka:` via mapstructure.
type Config struct {
	// Brokers is the list of Kafka broker addresses (host:port).
	Brokers []string `mapstructure:"brokers" json:"brokers"`

	// GroupID is the consumer group identifier. All consumers in the same group
	// share the partition assignment for a topic.
	GroupID string `mapstructure:"group_id" json:"group_id"`

	// Topic is the default topic for both producer writes and consumer reads.
	Topic string `mapstructure:"topic" json:"topic"`

	// DLQTopic is the dead-letter queue topic. When set, messages whose handler
	// fails after all retries are forwarded here with failure metadata headers
	// (x-original-topic, x-original-offset, x-error, x-failed-at).
	// When empty, failed messages are only logged.
	DLQTopic string `mapstructure:"dlq_topic" json:"dlq_topic"`

	// MaxRetries is the number of handler retries before a message is sent to
	// the DLQ (or logged if DLQTopic is empty). Defaults to 3.
	MaxRetries int `mapstructure:"max_retries" json:"max_retries"`

	// RetryBackoff is the base delay between retries. The actual delay for
	// attempt n is RetryBackoff * n (linear backoff). Defaults to 1 s.
	RetryBackoff time.Duration `mapstructure:"retry_backoff" json:"retry_backoff"`

	// CommitInterval controls how often the reader commits offsets to Kafka.
	// 0 means synchronous (manual) commit after every message.
	CommitInterval time.Duration `mapstructure:"commit_interval" json:"commit_interval"`

	// MinBytes is the minimum number of bytes to fetch per request.
	MinBytes int `mapstructure:"min_bytes" json:"min_bytes"`

	// MaxBytes is the maximum number of bytes to fetch per request.
	MaxBytes int `mapstructure:"max_bytes" json:"max_bytes"`

	// MaxWait is the maximum time the broker waits before returning a fetch
	// response even if MinBytes has not been reached.
	MaxWait time.Duration `mapstructure:"max_wait" json:"max_wait"`

	// Async enables fire-and-forget writes on the producer.
	// When true, Publish returns immediately without waiting for broker acks.
	// This increases throughput but sacrifices durability guarantees.
	Async bool `mapstructure:"async" json:"async"`
}

// Message is the transport type shared by Producer and Consumer.
type Message struct {
	// Topic overrides the producer's default topic for this specific message.
	// Leave empty to use the topic configured in Config.
	Topic string

	// Key is used by Kafka to assign the message to a partition.
	// Messages with the same key always go to the same partition,
	// preserving per-key ordering.
	Key []byte

	// Value is the message payload.
	Value []byte

	// Headers are optional key-value metadata forwarded as Kafka record headers.
	// Useful for trace IDs, schema versions, or routing information.
	Headers map[string]string

	// Offset is the position of the message in the partition (consumer-side only).
	Offset int64

	// Partition is the partition number the message was read from (consumer-side only).
	Partition int

	// Time is the timestamp set by the producer or the broker (consumer-side only).
	Time time.Time
}

// Handler is the callback invoked by Consumer.Subscribe for each received message.
// Returning a non-nil error triggers a retry according to Config.MaxRetries.
// After all retries are exhausted, the message is forwarded to the DLQ (if configured)
// or logged as an error.
type Handler func(ctx context.Context, msg Message) error

// Producer publishes messages to Kafka.
type Producer interface {
	// Publish sends one or more messages. A batch call is more efficient than
	// individual calls because kafka-go writes them in a single request.
	Publish(ctx context.Context, msgs ...Message) error

	// Close flushes buffered messages (important for async producers) and
	// releases the underlying connection. Must be called before process exit.
	Close() error
}

// Consumer reads messages from a Kafka topic and dispatches them to a Handler.
type Consumer interface {
	// Subscribe starts a blocking read loop. For each fetched message it:
	//   1. Calls handler, retrying up to Config.MaxRetries times on error.
	//   2. Forwards the message to the DLQ if all retries fail.
	//   3. Commits the offset so the message is not redelivered.
	//
	// Subscribe returns nil when ctx is cancelled (graceful shutdown).
	// It returns an error only on unexpected reader failures.
	Subscribe(ctx context.Context, handler Handler) error

	// Close shuts down the reader and DLQ writer (if configured).
	Close() error
}

// Client combines Producer, Consumer, and a health check Ping into a single
// handle returned by NewClient. It is the type stored in Engine.KafkaProducer
// and Engine.KafkaConsumer.
type Client interface {
	Producer
	Consumer
	// Ping verifies that at least one configured broker is reachable.
	// Used by the health check subsystem.
	Ping(ctx context.Context) error
}
