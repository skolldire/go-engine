package full_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/skolldire/go-engine/aws/provider/sqs"
	"github.com/skolldire/go-engine/http/provider/rest"
	"github.com/skolldire/go-engine/pkg/engine"
	presetfull "github.com/skolldire/go-engine/preset/full"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "application.yaml"), []byte(body), 0o600))
	return dir
}

const config = `
log:
  level: error
router:
  port: "0"
aws:
  region: "us-east-1"
rest:
  - payments:
      base_url: "http://localhost:9999"
  - billing:
      base_url: "http://localhost:9998"
sqs_clients:
  - orders:
      endpoint: "http://localhost:4566"
`

// TestFullPreset_DiscoversEveryDeclaredComponent verifies the preset turns the
// configuration file into providers without the application naming any of them.
func TestFullPreset_DiscoversEveryDeclaredComponent(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-1")

	dir := writeConfig(t, config)

	eng, err := engine.New(context.Background(),
		append([]engine.Option{engine.WithConfigDir(dir)}, presetfull.Options()...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	assert.ElementsMatch(t,
		[]string{"rest:payments", "rest:billing", "sqs:orders"},
		eng.ComponentNames(),
		"only the components declared in the file must be built")
}

// TestFullPreset_TypedRetrieval is the replacement for the old per-adapter
// getters on Engine: the type travels with the caller, so the core needs no
// method (and no import) per adapter.
func TestFullPreset_TypedRetrieval(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-1")

	dir := writeConfig(t, config)

	eng, err := engine.New(context.Background(),
		append([]engine.Option{engine.WithConfigDir(dir)}, presetfull.Options()...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	payments, err := rest.From(eng, "payments")
	require.NoError(t, err)
	assert.NotNil(t, payments)

	orders, err := sqs.From(eng, "orders")
	require.NoError(t, err)
	assert.NotNil(t, orders)

	_, err = rest.From(eng, "does-not-exist")
	assert.ErrorContains(t, err, "not found")
}

// TestFullPreset_UndeclaredComponentsAreNotBuilt is the behavioural half of the
// decoupling: a service that declares no Mongo section pays for no Mongo
// client, even under the all-in-one preset.
func TestFullPreset_UndeclaredComponentsAreNotBuilt(t *testing.T) {
	dir := writeConfig(t, `
log:
  level: error
rest:
  - api:
      base_url: "http://localhost:9999"
`)

	eng, err := engine.New(context.Background(),
		append([]engine.Option{engine.WithConfigDir(dir)}, presetfull.Options()...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	assert.Equal(t, []string{"rest:api"}, eng.ComponentNames())
}
