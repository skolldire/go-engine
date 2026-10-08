//go:build e2e

package test

import (
	"fmt"
	"testing"
	"time"

	mongoprovider "github.com/skolldire/go-engine/database/mongodb/provider/mongodb"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestEngine_MongoDB proves the provider reaches a real server. The client
// connects eagerly, so a wrong URI is a build-time failure the mocked tests
// cannot distinguish from success.
func TestEngine_MongoDB(t *testing.T) {
	requireDocker(t)

	ctx := ctxWithTimeout(t, 3*time.Minute)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "mongo:7",
			ExposedPorts: []string{"27017/tcp"},
			WaitingFor:   wait.ForLog("Waiting for connections").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err)
	terminate(t, container)

	uri := fmt.Sprintf("mongodb://%s", hostAddr(t, container, "27017/tcp"))

	dir := writeConfig(t, fmt.Sprintf(`
log:
  level: error
health:
  timeout: 5s
mongodb_clients:
  - main:
      uri: %q
      database: "engine_test"
      timeout: 20s
`, uri))

	eng := newEngine(t, dir,
		engine.WithHealth(),
		engine.WithProvider(mongoprovider.New("main")))

	client, err := mongoprovider.From(eng, "main")
	require.NoError(t, err)
	assert.NotNil(t, client, "the provider must expose a connected client")
}
