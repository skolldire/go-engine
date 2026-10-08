package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingTelemetry counts what providers sent through it.
type recordingTelemetry struct {
	spans    []string
	counters []string
}

func (r *recordingTelemetry) Span(ctx context.Context, name string, fn func(context.Context) error) error {
	r.spans = append(r.spans, name)
	return fn(ctx)
}
func (r *recordingTelemetry) Counter(_ context.Context, name string, _ int64) {
	r.counters = append(r.counters, name)
}
func (r *recordingTelemetry) Histogram(_ context.Context, _ string, _ float64) {}

// telemetrySourceProvider mimics provider/otel: it supplies the engine's
// telemetry implementation from inside Init.
type telemetrySourceProvider struct {
	impl *recordingTelemetry
}

func (t *telemetrySourceProvider) Name() string      { return "telemetry" }
func (t *telemetrySourceProvider) ConfigKey() string { return "telemetry" }
func (t *telemetrySourceProvider) Init(context.Context, RawConfig, Deps) (any, error) {
	t.impl = &recordingTelemetry{}
	return t.impl, nil
}
func (t *telemetrySourceProvider) Close(context.Context) error { return nil }
func (t *telemetrySourceProvider) Telemetry() Telemetry        { return t.impl }

// telemetryUsingProvider records through whatever Deps handed it, keeping the
// reference the way a real adapter would.
type telemetryUsingProvider struct {
	name string
	tel  Telemetry
}

func (u *telemetryUsingProvider) Name() string      { return u.name }
func (u *telemetryUsingProvider) ConfigKey() string { return u.name }
func (u *telemetryUsingProvider) Init(_ context.Context, _ RawConfig, deps Deps) (any, error) {
	u.tel = deps.Telemetry
	return u, nil
}
func (u *telemetryUsingProvider) Close(context.Context) error { return nil }

func tempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "application.yaml"), []byte(body), 0o600))
	return dir
}

// TestTelemetry_ReachesProvidersBuiltBeforeIt is the regression test for the
// telemetry provider being built but never wired: providers assembled earlier
// captured the no-op and their spans vanished silently.
func TestTelemetry_ReachesProvidersBuiltBeforeIt(t *testing.T) {
	dir := tempConfig(t, "log:\n  level: error\n")

	earlier := &telemetryUsingProvider{name: "earlier"}
	source := &telemetrySourceProvider{}
	later := &telemetryUsingProvider{name: "later"}

	eng, err := New(context.Background(), WithConfigDir(dir),
		WithProvider(earlier, source, later))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	ctx := context.Background()
	require.NoError(t, earlier.tel.Span(ctx, "from-earlier", func(context.Context) error { return nil }))
	require.NoError(t, later.tel.Span(ctx, "from-later", func(context.Context) error { return nil }))

	assert.Equal(t, []string{"from-earlier", "from-later"}, source.impl.spans,
		"a provider registered before the telemetry provider must still record through it")
}

func TestTelemetry_DefaultsToNoop(t *testing.T) {
	dir := tempConfig(t, "log:\n  level: error\n")

	user := &telemetryUsingProvider{name: "user"}
	eng, err := New(context.Background(), WithConfigDir(dir), WithProvider(user))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	require.NotNil(t, user.tel, "Deps.Telemetry is never nil")
	assert.NotPanics(t, func() {
		_ = user.tel.Span(context.Background(), "s", func(context.Context) error { return nil })
		user.tel.Counter(context.Background(), "c", 1)
		user.tel.Histogram(context.Background(), "h", 1)
	})
	assert.NotNil(t, eng.Telemetry())
}

// TestTelemetry_ExplicitOverrideWins covers engine.WithTelemetry.
func TestTelemetry_ExplicitOverrideWins(t *testing.T) {
	dir := tempConfig(t, "log:\n  level: error\n")

	explicit := &recordingTelemetry{}
	user := &telemetryUsingProvider{name: "user"}

	eng, err := New(context.Background(), WithConfigDir(dir),
		WithTelemetry(explicit), WithProvider(user))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	require.NoError(t, user.tel.Span(context.Background(), "explicit", func(context.Context) error { return nil }))
	assert.Equal(t, []string{"explicit"}, explicit.spans)
}
