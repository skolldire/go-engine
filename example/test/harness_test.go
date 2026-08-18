//go:build e2e

// Package e2e validates go-engine against real backing services.
//
// The suite builds a real engine from a real configuration file and drives it
// against containers, because that is the only way to catch the failures the
// unit tests structurally cannot: a configuration key that maps to nothing, a
// client that never actually connects, a health check that reports ready while
// its dependency is down.
package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

// dockerOnce resolves the Docker endpoint once for the whole suite.
var dockerOnce sync.Once

// resolveDockerHost points testcontainers at the active Docker context when
// DOCKER_HOST is unset.
//
// Docker Desktop puts its socket where the library looks by default; Colima,
// Rancher and podman do not, and testcontainers does not consult `docker
// context` itself. Without this, a perfectly working daemon looks absent.
func resolveDockerHost(t *testing.T) {
	t.Helper()

	dockerOnce.Do(func() {
		if os.Getenv("DOCKER_HOST") != "" {
			return
		}

		out, err := exec.Command("docker", "context", "inspect",
			"--format", "{{.Endpoints.docker.Host}}").Output()
		if err != nil {
			return // leave it to the library's own discovery
		}

		if host := strings.TrimSpace(string(out)); host != "" {
			_ = os.Setenv("DOCKER_HOST", host)
			// Ryuk, the resource reaper, runs *inside* the VM and bind-mounts
			// the Docker socket. It must be given the path as seen from there,
			// not the host path: Colima exposes the socket at
			// ~/.colima/... on the host but /var/run/docker.sock inside, and
			// mounting the host path fails with "operation not supported".
			if strings.HasPrefix(host, "unix://") &&
				!strings.HasPrefix(host, "unix:///var/run/") {
				_ = os.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
			}
		}
	})
}

// requireDocker skips the test when no Docker daemon is reachable.
//
// Skipping rather than failing is deliberate: this suite is opt-in through the
// e2e build tag, and a developer without Docker should still get a green run.
//
// The recover is not defensive padding: testcontainers panics rather than
// returning an error when it cannot locate a socket, so without it an absent
// daemon crashes the run instead of skipping it.
func requireDocker(t *testing.T) {
	t.Helper()

	resolveDockerHost(t)

	var provider *testcontainers.DockerProvider

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("Docker not available: %v", r)
			}
		}()

		var err error
		provider, err = testcontainers.NewDockerProvider()
		if err != nil {
			t.Skipf("no Docker provider: %v", err)
		}
	}()

	if provider == nil {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := provider.Health(ctx); err != nil {
		t.Skipf("Docker daemon not reachable: %v", err)
	}
}

// writeConfig materialises a configuration file and returns its directory, so
// the engine loads it exactly as a deployed application would rather than being
// handed a struct.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "application.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	return dir
}

// newEngine builds an engine from the given configuration and registers it for
// shutdown, failing the test if anything is left open.
func newEngine(t *testing.T, configDir string, opts ...engine.Option) *engine.Engine {
	t.Helper()
	return newEngineWithShutdown(t, configDir, true, opts...)
}

// newEngineWithShutdown is newEngine with control over the shutdown assertion.
//
// A test that deliberately forces a drain — cutting an in-flight RPC short, for
// instance — makes Close report that forcing, correctly. Asserting a clean
// shutdown there would be asserting the opposite of what the test set up.
func newEngineWithShutdown(
	t *testing.T,
	configDir string,
	requireCleanShutdown bool,
	opts ...engine.Option,
) *engine.Engine {
	t.Helper()

	all := append([]engine.Option{engine.WithConfigDir(configDir)}, opts...)

	eng, err := engine.New(context.Background(), all...)
	require.NoError(t, err, "the engine must build against real services")

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := eng.Close(ctx)
		if requireCleanShutdown {
			require.NoError(t, err, "shutdown must release every component cleanly")
			return
		}
		// Still must not hang or panic; the error itself is expected.
		t.Logf("shutdown reported: %v", err)
	})

	return eng
}

// ctxWithTimeout bounds every container interaction so a hung backend fails the
// test instead of the whole suite.
func ctxWithTimeout(t *testing.T, d time.Duration) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)

	return ctx
}

// terminate registers a container for cleanup, reporting a failure to stop it
// rather than leaking it into the developer's machine.
func terminate(t *testing.T, c testcontainers.Container) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.Terminate(ctx); err != nil {
			t.Errorf("terminating container: %v", err)
		}
	})
}

// hostPort returns the host address and port a test can dial for a container
// port such as "6379/tcp".
func hostPort(t *testing.T, c testcontainers.Container, port nat.Port) (string, int) {
	t.Helper()

	ctx := ctxWithTimeout(t, 30*time.Second)

	host, err := c.Host(ctx)
	require.NoError(t, err)

	mapped, err := c.MappedPort(ctx, port)
	require.NoError(t, err)

	return host, mapped.Int()
}

// hostAddr is hostPort formatted as host:port.
func hostAddr(t *testing.T, c testcontainers.Container, port nat.Port) string {
	t.Helper()

	host, p := hostPort(t, c, port)
	return fmt.Sprintf("%s:%d", host, p)
}
