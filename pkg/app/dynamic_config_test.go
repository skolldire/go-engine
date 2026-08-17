package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// writeConfig points CONF_DIR at a temp directory holding application.yaml and
// returns the file path so tests can rewrite it to trigger a reload.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "application.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	t.Setenv("CONF_DIR", dir)
	t.Setenv("SCOPE", "local")

	return path
}

const baseConfig = `
enable_config_watch: true
log:
  level: info
feature_flags:
  beta: false
`

const reloadedConfig = `
enable_config_watch: true
log:
  level: info
feature_flags:
  beta: true
`

// TestWithDynamicConfig_PublishesReload is the regression test for the watcher
// storing reloaded configuration in an instance nobody could reach: Engine.Conf
// stayed pinned to the build-time snapshot, so no consumer ever observed a
// reload despite the README recommending this path.
func TestWithDynamicConfig_PublishesReload(t *testing.T) {
	path := writeConfig(t, baseConfig)

	builder := NewAppBuilder().WithContext(context.Background()).WithDynamicConfig()
	require.Empty(t, builder.GetErrors())

	engine := builder.engine
	t.Cleanup(func() { _ = engine.Close(context.Background()) })

	require.NotNil(t, engine.Config())
	require.NotNil(t, engine.DynamicConfig())
	assert.Equal(t, false, engine.Config().FeatureFlags["beta"])

	require.NoError(t, os.WriteFile(path, []byte(reloadedConfig), 0o600))

	assert.Eventually(t, func() bool {
		return engine.Config().FeatureFlags["beta"] == true
	}, 10*time.Second, 50*time.Millisecond,
		"Config() must return the reloaded values, not the build-time snapshot")
}

// TestWithDynamicConfig_PopulatesFeatureFlags covers the third symptom: this
// path left GetFeatureFlags returning nil because only WithInitialization
// populated it.
func TestWithDynamicConfig_PopulatesFeatureFlags(t *testing.T) {
	writeConfig(t, baseConfig)

	builder := NewAppBuilder().WithContext(context.Background()).WithDynamicConfig()
	require.Empty(t, builder.GetErrors())

	engine := builder.engine
	t.Cleanup(func() { _ = engine.Close(context.Background()) })

	require.NotNil(t, engine.GetFeatureFlags(),
		"WithDynamicConfig alone must populate the feature flags")
	assert.False(t, engine.GetFeatureFlags().GetBool("beta"))
}

// TestWithDynamicConfig_FeatureFlagsFollowReload verifies the flags instance is
// updated in place rather than swapped, so a consumer holding the pointer sees
// new values.
func TestWithDynamicConfig_FeatureFlagsFollowReload(t *testing.T) {
	path := writeConfig(t, baseConfig)

	builder := NewAppBuilder().WithContext(context.Background()).WithDynamicConfig()
	require.Empty(t, builder.GetErrors())

	engine := builder.engine
	t.Cleanup(func() { _ = engine.Close(context.Background()) })

	flags := engine.GetFeatureFlags()
	require.NotNil(t, flags)
	require.False(t, flags.GetBool("beta"))

	require.NoError(t, os.WriteFile(path, []byte(reloadedConfig), 0o600))

	assert.Eventually(t, func() bool {
		return flags.GetBool("beta")
	}, 10*time.Second, 50*time.Millisecond,
		"the flags instance handed to consumers must follow reloads")
}

// TestWithDynamicConfig_CloseStopsWatcher is the regression test for the leaked
// watcher goroutine: DynamicConfig was a local variable, so Stop was
// unreachable and the fsnotify goroutines lived for the process lifetime.
func TestWithDynamicConfig_CloseStopsWatcher(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
	)

	writeConfig(t, baseConfig)

	builder := NewAppBuilder().WithContext(context.Background()).WithDynamicConfig()
	require.Empty(t, builder.GetErrors())

	engine := builder.engine
	require.NoError(t, engine.Close(context.Background()))

	// Give the watcher goroutines a moment to unwind after Stop closed fsnotify.
	time.Sleep(200 * time.Millisecond)
}
