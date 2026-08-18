//go:build e2e

package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/skolldire/go-engine/messaging/pkg/integration/rabbitmq"
	grpcprovider "github.com/skolldire/go-engine/messaging/provider/grpcclient"
	grpcsrvprovider "github.com/skolldire/go-engine/messaging/provider/grpcserver"
	kafkaprovider "github.com/skolldire/go-engine/messaging/provider/kafka"
	rabbitprovider "github.com/skolldire/go-engine/messaging/provider/rabbitmq"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc/connectivity"
)

// TestEngine_RabbitMQ drives the provider against a real broker: publishing,
// consuming, and the panic containment that only matters once a real delivery
// reaches a handler.
func TestEngine_RabbitMQ(t *testing.T) {
	requireDocker(t)

	ctx := ctxWithTimeout(t, 5*time.Minute)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "rabbitmq:3-alpine",
			ExposedPorts: []string{"5672/tcp"},
			WaitingFor: wait.ForLog("Server startup complete").
				WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err)
	terminate(t, container)

	uri := fmt.Sprintf("amqp://guest:guest@%s/", hostAddr(t, container, "5672/tcp"))

	dir := writeConfig(t, fmt.Sprintf(`
log:
  level: error
rabbitmq_clients:
  - events:
      url: %q
      timeout: 30s
`, uri))

	eng := newEngine(t, dir, engine.WithProvider(rabbitprovider.New("events")))

	client, err := rabbitprovider.From(eng, "events")
	require.NoError(t, err)

	const queue = "e2e-orders"
	require.NoError(t, client.DeclareQueue(ctx, queue, true, false, false, false, nil))

	t.Run("publishes and consumes", func(t *testing.T) {
		received := make(chan string, 1)

		require.NoError(t, client.Consume(ctx, queue, false, func(d amqp.Delivery) error {
			received <- string(d.Body)
			return nil
		}))

		require.NoError(t, client.Publish(ctx, rabbitmq.Message{RoutingKey: queue, Body: []byte(`{"order":1}`)}))

		select {
		case body := <-received:
			assert.Contains(t, body, `"order":1`)
		case <-time.After(30 * time.Second):
			t.Fatal("the published message never reached the consumer")
		}
	})

	// A panicking handler used to unwind the consumer goroutine, so the queue
	// stopped being read entirely. This is the check that it now survives.
	t.Run("a panicking handler does not kill the consumer", func(t *testing.T) {
		const panicQueue = "e2e-panic"
		require.NoError(t, client.DeclareQueue(ctx, panicQueue, true, false, false, false, nil))

		var once sync.Once
		survived := make(chan string, 1)

		require.NoError(t, client.Consume(ctx, panicQueue, true, func(d amqp.Delivery) error {
			first := false
			once.Do(func() { first = true })
			if first {
				panic("first delivery explodes")
			}
			survived <- string(d.Body)
			return nil
		}))

		require.NoError(t, client.Publish(ctx, rabbitmq.Message{RoutingKey: panicQueue, Body: []byte("boom")}))
		time.Sleep(time.Second)
		require.NoError(t, client.Publish(ctx, rabbitmq.Message{RoutingKey: panicQueue, Body: []byte("after")}))

		select {
		case body := <-survived:
			assert.Equal(t, "after", body,
				"the consumer must still be reading after a handler panicked")
		case <-time.After(30 * time.Second):
			t.Fatal("the consumer died on the panicking delivery")
		}
	})
}

// TestEngine_Kafka drives the provider against a real broker.
func TestEngine_Kafka(t *testing.T) {
	requireDocker(t)

	ctx := ctxWithTimeout(t, 6*time.Minute)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "apache/kafka:3.8.0",
			ExposedPorts: []string{"9092/tcp"},
			Env: map[string]string{
				"KAFKA_NODE_ID":                                  "1",
				"KAFKA_PROCESS_ROLES":                            "broker,controller",
				"KAFKA_CONTROLLER_QUORUM_VOTERS":                 "1@localhost:9093",
				"KAFKA_LISTENERS":                                "PLAINTEXT://:9092,CONTROLLER://:9093",
				"KAFKA_ADVERTISED_LISTENERS":                     "PLAINTEXT://localhost:9092",
				"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":           "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT",
				"KAFKA_CONTROLLER_LISTENER_NAMES":                "CONTROLLER",
				"KAFKA_INTER_BROKER_LISTENER_NAME":               "PLAINTEXT",
				"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
				"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS":         "0",
				"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
				"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
			},
			WaitingFor: wait.ForLog("Kafka Server started").WithStartupTimeout(4 * time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err)
	terminate(t, container)

	broker := hostAddr(t, container, "9092/tcp")

	dir := writeConfig(t, fmt.Sprintf(`
log:
  level: error
kafka:
  brokers:
    - %q
  topic: "e2e-events"
  group_id: "e2e-group"
`, broker))

	eng := newEngine(t, dir, engine.WithProvider(kafkaprovider.New()))

	client, err := kafkaprovider.From(eng)
	require.NoError(t, err)
	assert.NotNil(t, client, "the provider must expose a connected client")
}

// TestEngine_GRPC covers the client and the server together, including the
// bounded shutdown: a drain that cannot finish must not hold the engine open.
func TestEngine_GRPC(t *testing.T) {
	dir := writeConfig(t, `
log:
  level: error
grpc_server:
  puerto: 0
grpc_client:
  - auth:
      target: "127.0.0.1:1"
      timeout: 2s
`)

	eng := newEngine(t, dir, engine.WithProvider(
		grpcsrvprovider.New(),
		grpcprovider.New("auth"),
	))

	t.Run("the client builds lazily against an unreachable target", func(t *testing.T) {
		client, err := grpcprovider.From(eng, "auth")
		require.NoError(t, err, "a lazy connection must not fail on an unreachable target")
		assert.NotEqual(t, connectivity.Shutdown, client.CheckConnection())
	})

	t.Run("the server is built", func(t *testing.T) {
		srv, err := grpcsrvprovider.From(eng)
		require.NoError(t, err)
		assert.NotNil(t, srv)
	})

	// The engine closes its components under a deadline; this is the check that
	// the gRPC server honours it instead of blocking on GracefulStop.
	t.Run("shutdown finishes within the deadline", func(t *testing.T) {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() { done <- eng.Close(shutdownCtx) }()

		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatal("shutdown must respect its deadline")
		}
	})
}
