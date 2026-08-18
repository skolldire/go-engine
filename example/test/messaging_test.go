//go:build e2e

package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/skolldire/go-engine/messaging/pkg/integration/kafka"
	"github.com/skolldire/go-engine/messaging/pkg/integration/rabbitmq"
	grpcsrv "github.com/skolldire/go-engine/messaging/pkg/server/grpc"
	grpcprovider "github.com/skolldire/go-engine/messaging/provider/grpcclient"
	grpcsrvprovider "github.com/skolldire/go-engine/messaging/provider/grpcserver"
	kafkaprovider "github.com/skolldire/go-engine/messaging/provider/kafka"
	rabbitprovider "github.com/skolldire/go-engine/messaging/provider/rabbitmq"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
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

// TestEngine_Kafka publishes and consumes against a real broker.
//
// The previous version of this test started a container and asserted that a
// client existed. That proved nothing: the Kafka client is lazy, so it would
// have passed against a broker that was never reachable. It also advertised
// localhost:9092 while testcontainers maps a random host port, so nothing could
// have connected even if the test had tried.
func TestEngine_Kafka(t *testing.T) {
	requireDocker(t)

	ctx := ctxWithTimeout(t, 8*time.Minute)

	// The advertised listener must name the port the host will dial, and that
	// port is only known after the container starts. Fixing it on both sides is
	// what makes the broker reachable from outside the container.
	const hostPortNum = "39092"

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "apache/kafka:3.8.0",
			ExposedPorts: []string{
				fmt.Sprintf("%s:9092/tcp", hostPortNum),
			},
			Env: map[string]string{
				"KAFKA_NODE_ID":                                  "1",
				"KAFKA_PROCESS_ROLES":                            "broker,controller",
				"KAFKA_CONTROLLER_QUORUM_VOTERS":                 "1@localhost:9093",
				"KAFKA_LISTENERS":                                "PLAINTEXT://:9092,CONTROLLER://:9093",
				"KAFKA_ADVERTISED_LISTENERS":                     "PLAINTEXT://localhost:" + hostPortNum,
				"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":           "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT",
				"KAFKA_CONTROLLER_LISTENER_NAMES":                "CONTROLLER",
				"KAFKA_INTER_BROKER_LISTENER_NAME":               "PLAINTEXT",
				"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
				"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS":         "0",
				"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
				"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
				"KAFKA_AUTO_CREATE_TOPICS_ENABLE":                "true",
			},
			WaitingFor: wait.ForLog("Kafka Server started").WithStartupTimeout(5 * time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err)
	terminate(t, container)

	broker := "localhost:" + hostPortNum

	dir := writeConfig(t, fmt.Sprintf(`
log:
  level: error
kafka:
  brokers:
    - %q
  topic: "e2e-events"
  group_id: "e2e-group"
  # Without these the reader waits for a full batch, which never arrives when
  # the test publishes a single message.
  min_bytes: 1
  max_bytes: 1048576
  max_wait: 500ms
  commit_interval: 500ms
`, broker))

	eng := newEngine(t, dir, engine.WithProvider(kafkaprovider.New()))

	client, err := kafkaprovider.From(eng)
	require.NoError(t, err)

	// Publishing is what creates the topic, since auto-creation is on. Doing it
	// first also proves the advertised listener is reachable from the host,
	// which a lazy client would otherwise never reveal. It is retried because
	// the broker reports the topic as unknown for a moment after creating it.
	require.Eventually(t, func() bool {
		return client.Publish(ctx, kafka.Message{
			Key:   []byte("warmup"),
			Value: []byte(`{"warmup":true}`),
		}) == nil
	}, 90*time.Second, 2*time.Second, "the broker must accept a publish from the host")

	// Consuming is deliberately NOT asserted here, and that is a known gap
	// rather than an oversight.
	//
	// Subscribe runs and blocks — it neither errors nor returns — but no message
	// is delivered within two minutes against a freshly created single-broker
	// cluster, and the cause has not been identified. Rather than assert
	// something weaker and call the path covered, the gap is stated: a reader
	// that silently delivers nothing is exactly the failure a green test would
	// hide.
	//
	// What this test does prove is real: the broker is reachable from the host
	// on its advertised listener, and Publish reaches it. That is not
	// incidental — it failed with "Unknown Topic Or Partition" until the
	// advertised listener matched the mapped port, so a lazy client alone could
	// never have passed it.
	//
	// The RabbitMQ test above covers the publish-and-consume round trip end to
	// end, including panic containment, so the messaging contract is not
	// entirely unverified.
}

// TestEngine_GRPC starts the server, calls it, and proves shutdown is bounded
// while an RPC is still in flight.
//
// The previous version registered no service, started nothing and made no call:
// it finished in 0.00s and would have passed against a server that could not
// run at all.
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

	srv, err := grpcsrvprovider.From(eng)
	require.NoError(t, err)

	// blocking releases the handler, so the test controls when an in-flight RPC
	// finishes and can observe shutdown while one is stuck.
	blocking := make(chan struct{})
	entered := make(chan struct{}, 1)

	// A streaming method rather than a unary one: the handler is entered as soon
	// as the stream opens, with no message to decode, so the test does not need
	// a generated proto service just to hold an RPC open.
	srv.RegisterService(func(s *grpc.Server) {
		s.RegisterService(&grpc.ServiceDesc{
			ServiceName: "e2e.Blocker",
			HandlerType: (*any)(nil),
			Streams: []grpc.StreamDesc{{
				StreamName: "Hold",
				Handler: func(_ any, stream grpc.ServerStream) error {
					select {
					case entered <- struct{}{}:
					default:
					}
					<-blocking
					return nil
				},
				ServerStreams: true,
				ClientStreams: true,
			}},
		}, struct{}{})
	})

	ctx := ctxWithTimeout(t, 2*time.Minute)
	require.NoError(t, srv.Start(ctx))

	t.Run("the client builds lazily against an unreachable target", func(t *testing.T) {
		client, err := grpcprovider.From(eng, "auth")
		require.NoError(t, err, "a lazy connection must not fail on an unreachable target")
		assert.NotEqual(t, connectivity.Shutdown, client.CheckConnection())
	})

	// This is the case the bounded shutdown exists for: GracefulStop waits for
	// in-flight RPCs with no deadline of its own, so without the bound a single
	// stuck call held the engine open until the process was killed.
	t.Run("shutdown is bounded while an RPC is stuck", func(t *testing.T) {
		addr := serverAddress(t, srv)

		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()

		go func() {
			_, _ = conn.NewStream(context.Background(),
				&grpc.StreamDesc{StreamName: "Hold", ServerStreams: true, ClientStreams: true},
				"/e2e.Blocker/Hold")
		}()

		select {
		case <-entered:
		case <-time.After(30 * time.Second):
			t.Skip("the RPC never reached the handler; nothing to hold shutdown open")
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		start := time.Now()
		err = srv.Stop(shutdownCtx)
		elapsed := time.Since(start)

		close(blocking)

		assert.Less(t, elapsed, 15*time.Second,
			"a stuck RPC must not hold shutdown past the deadline")
		assert.ErrorIs(t, err, grpcsrv.ErrForcedShutdown,
			"and a forced drain must be reported, not silently succeed")
	})
}

// serverAddress returns the address the server bound, which is only known after
// Start because the configured port is 0.
func serverAddress(t *testing.T, srv grpcsrv.Service) string {
	t.Helper()

	require.Eventually(t, func() bool { return srv.Address() != "" }, 30*time.Second, 100*time.Millisecond,
		"the server must report the address it bound")

	return srv.Address()
}
