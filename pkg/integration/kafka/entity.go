package kafka

import (
	"context"
	"time"
)

type Config struct {
	Brokers        []string      `mapstructure:"brokers"         json:"brokers"`
	GroupID        string        `mapstructure:"group_id"        json:"group_id"`
	Topic          string        `mapstructure:"topic"           json:"topic"`
	DLQTopic       string        `mapstructure:"dlq_topic"       json:"dlq_topic"`
	MaxRetries     int           `mapstructure:"max_retries"     json:"max_retries"`
	RetryBackoff   time.Duration `mapstructure:"retry_backoff"   json:"retry_backoff"`
	CommitInterval time.Duration `mapstructure:"commit_interval" json:"commit_interval"`
	MinBytes       int           `mapstructure:"min_bytes"       json:"min_bytes"`
	MaxBytes       int           `mapstructure:"max_bytes"       json:"max_bytes"`
	MaxWait        time.Duration `mapstructure:"max_wait"        json:"max_wait"`
	Async          bool          `mapstructure:"async"           json:"async"`
}

type Message struct {
	Topic     string
	Key       []byte
	Value     []byte
	Headers   map[string]string
	Offset    int64
	Partition int
	Time      time.Time
}

type Handler func(ctx context.Context, msg Message) error

type Producer interface {
	Publish(ctx context.Context, msgs ...Message) error
	Close() error
}

type Consumer interface {
	Subscribe(ctx context.Context, handler Handler) error
	Close() error
}

type Client interface {
	Producer
	Consumer
	Ping(ctx context.Context) error
}
