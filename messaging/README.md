# go-engine/messaging

Kafka, RabbitMQ, gRPC client, and gRPC server for go-engine.

```bash
go get github.com/skolldire/go-engine
```

---

## Kafka

Built on [segmentio/kafka-go](https://github.com/segmentio/kafka-go) v0.4.51. `NewClient` returns a single handle that implements both `Producer` and `Consumer`.

```yaml
kafka:
  brokers: ["kafka:9092"]
  group_id: "assessment-service"
  topic: "exam-events"
  dlq_topic: "exam-events-dlq"   # empty → log failed messages instead
  max_retries: 3
  retry_backoff: 1s
  commit_interval: 0             # 0 = synchronous commit per message
  async: false
```

### Publishing

```go
producer := engine.GetKafkaProducer()

err := producer.Publish(ctx,
    kafka.Message{
        Key:   []byte(examID),          // same key → same partition → ordered
        Value: jsonBytes,
        Headers: map[string]string{
            "event-type": "ExamCompleted",
            "trace-id":   traceID,
        },
    },
    // variadic: send multiple in one request
)
```

### Consuming

```go
consumer := engine.GetKafkaConsumer()

go func() {
    err := consumer.Subscribe(ctx, func(ctx context.Context, msg kafka.Message) error {
        var event ExamCompletedEvent
        if err := json.Unmarshal(msg.Value, &event); err != nil {
            return err // triggers retry → DLQ after MaxRetries
        }
        return processEvent(ctx, event)
    })
    // returns nil on context cancellation (graceful shutdown)
}()
```

**Consumer behaviour:**
- Retries handler up to `max_retries` with linear backoff (`retry_backoff × attempt`).
- On final failure → forwards to `dlq_topic` with headers `x-original-topic`, `x-original-offset`, `x-error`, `x-failed-at`. If no DLQ configured → logs the error.
- Commits offset after every message regardless of handler result (at-least-once delivery).
- Returns `nil` when `ctx` is cancelled — use this for graceful shutdown.

### Health check

```go
builder.RegisterHealthChecker("kafka", kafka.NewChecker(cfg.Brokers))
```

---

## RabbitMQ

```yaml
rabbitmq_clients:
  - events:
      url: "amqp://guest:guest@localhost:5672/"
      exchange: "events"
      exchange_type: "topic"
      queue: "assessment.events"
      routing_key: "exam.*"
      durable: true
      auto_ack: false
```

```go
import rabbitprovider "github.com/skolldire/go-engine/messaging/provider/rabbitmq"

// Registered at build time:
//   engine.New(ctx, engine.WithProvider(rabbitprovider.New("events")))
rb, err := rabbitprovider.From(eng, "events")
if err != nil {
	return err
}
err := rb.Publish(ctx, "exam.completed", jsonBytes)

go rb.Consume(ctx, func(msg rabbitmq.Message) error {
    return handle(msg)
})
```

---

## gRPC client

```yaml
grpc_client:
  - calibration:
      address: "calibration-service:50051"
      timeout: 10
      enable_logging: true
```

```go
import grpcprovider "github.com/skolldire/go-engine/messaging/provider/grpcclient"

// Registered at build time:
//   engine.New(ctx, engine.WithProvider(grpcprovider.New("calibration")))
conn, err := grpcprovider.From(eng, "calibration")
if err != nil {
    return err
}

// Get the underlying *grpc.ClientConn for generated stubs:
grpcConn := conn.GetConnection()
pbClient := pb.NewCalibrationServiceClient(grpcConn)

// Attach metadata to outgoing calls:
ctx = conn.WithHeaders(ctx, map[string]string{
    "authorization": "Bearer " + token,
})

// Check connectivity:
state := conn.CheckConnection()
conn.ReconnectIfNeeded(ctx)
```

**Available methods:** `GetConnection()`, `WithMetadata()`, `WithHeaders()`, `CheckConnection()`, `ReconnectIfNeeded()`, `Close()`.

---

## gRPC server

```yaml
grpc_server:
  puerto: 50051         # field name "puerto" (legacy Spanish naming)
  enable_logging: true
```

```go
import (
    grpcsrvprovider "github.com/skolldire/go-engine/messaging/provider/grpcserver"
    "github.com/skolldire/go-engine/pkg/engine"
)

// The provider reads the grpc_server section and builds the server:
eng, err := engine.New(ctx, engine.WithProvider(grpcsrvprovider.New()))
if err != nil {
    log.Fatal(err)
}

grpcSrv, err := grpcsrvprovider.From(eng)
if err != nil {
    log.Fatal(err)
}

// Register generated service implementations before Start:
grpcSrv.RegisterService(func(s *grpc.Server) {
    pb.RegisterCalibrationServiceServer(s, &myImpl{})
})

// Start is non-blocking; cancelling ctx triggers a drain bounded by
// shutdown_timeout. Start after Stop returns ErrServerStopped: a *grpc.Server
// is single-use.
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
if err := grpcSrv.Start(ctx); err != nil {
    log.Fatal(err)
}
```

### Adding interceptors

Interceptors must be supplied at construction: gRPC fixes them when the server
is created and offers no way to add them later. Pass them to the provider, which
forwards them to `grpc.NewServer`:

```go
import (
    grpcsrvprovider "github.com/skolldire/go-engine/messaging/provider/grpcserver"
    "google.golang.org/grpc"
)

eng, err := engine.New(ctx,
    engine.WithProvider(grpcsrvprovider.New(
        grpc.ChainUnaryInterceptor(myAuthInterceptor, myLoggingInterceptor),
        grpc.ChainStreamInterceptor(myStreamInterceptor),
    )),
)
if err != nil {
    log.Fatal(err)
}

srv, err := grpcsrvprovider.From(eng)
```

Any `grpc.ServerOption` works the same way — `MaxRecvMsgSize`, `Creds`,
`KeepaliveParams`. Building a bare `*grpc.Server` by hand is no longer
necessary, and costs more than it looks: it gives up the deadline-bounded
`Stop`, the restart and double-start checks, `Address()` for `puerto: 0`, and
the automatic reflection registration.

### Reflection

gRPC reflection is registered automatically, enabling `grpcurl` and Postman to discover services without pre-compiled proto descriptors.
