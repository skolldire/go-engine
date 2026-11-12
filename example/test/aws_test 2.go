//go:build e2e

package test

import (
	"fmt"
	"testing"
	"time"

	awspreset "github.com/skolldire/go-engine/aws/preset"
	s3provider "github.com/skolldire/go-engine/aws/provider/s3"
	sqsprovider "github.com/skolldire/go-engine/aws/provider/sqs"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestEngine_AWS drives the AWS family against LocalStack, through the preset's
// configuration-driven discovery rather than by naming providers by hand.
//
// This is the case the unit tests can least approximate: they construct clients
// against fake credentials and never make a call, so a wrong endpoint, an
// unshared credential chain or a mis-decoded section all look identical to
// success.
func TestEngine_AWS(t *testing.T) {
	requireDocker(t)

	ctx := ctxWithTimeout(t, 5*time.Minute)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "localstack/localstack:3",
			ExposedPorts: []string{"4566/tcp"},
			Env:          map[string]string{"SERVICES": "sqs,s3"},
			WaitingFor:   wait.ForLog("Ready.").WithStartupTimeout(4 * time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err)
	terminate(t, container)

	endpoint := "http://" + hostAddr(t, container, "4566/tcp")

	// LocalStack accepts any credentials, but the SDK still needs them present.
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-1")

	dir := writeConfig(t, fmt.Sprintf(`
log:
  level: error
health:
  timeout: 5s
aws:
  region: "us-east-1"
sqs_clients:
  - orders:
      endpoint: %q
s3_clients:
  - assets:
      region: "us-east-1"
      bucket: "e2e-assets"
`, endpoint))

	// Options() discovers both clients from the file: nothing here names them.
	eng := newEngine(t, dir, append([]engine.Option{engine.WithHealth()}, awspreset.Options()...)...)

	assert.ElementsMatch(t, []string{"sqs:orders", "s3:assets"}, eng.ComponentNames(),
		"the preset must build exactly the clients the file declares")

	t.Run("SQS round-trips a message", func(t *testing.T) {
		client, err := sqsprovider.From(eng, "orders")
		require.NoError(t, err)

		queueURL, err := client.CreateQueue(ctx, "e2e-orders", nil)
		require.NoError(t, err, "the client must reach LocalStack")

		_, err = client.SendMessage(ctx, queueURL, `{"order":1}`, nil)
		require.NoError(t, err)

		msgs, err := client.ReceiveMessages(ctx, queueURL, 1, 5)
		require.NoError(t, err)
		require.Len(t, msgs, 1, "the message just sent must come back")
		assert.Contains(t, *msgs[0].Body, `"order":1`)
	})

	t.Run("S3 client is usable", func(t *testing.T) {
		client, err := s3provider.From(eng, "assets")
		require.NoError(t, err)
		assert.NotNil(t, client)
	})
}
