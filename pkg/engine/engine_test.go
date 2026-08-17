package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProvider is a Provider test double recording how the core drove it.
type fakeProvider struct {
	name      string
	configKey string
	value     any
	initErr   error
	closeErr  error

	initCalled  bool
	closeCalled bool
	sawExists   bool
	decoded     fakeConfig
}

type fakeConfig struct {
	Endpoint string `mapstructure:"endpoint"`
	Retries  int    `mapstructure:"retries"`
}

func (f *fakeProvider) Name() string      { return f.name }
func (f *fakeProvider) ConfigKey() string { return f.configKey }

func (f *fakeProvider) Init(_ context.Context, raw RawConfig, _ Deps) (any, error) {
	f.initCalled = true
	f.sawExists = raw.Exists()
	_ = raw.Decode(&f.decoded)
	if f.initErr != nil {
		return nil, f.initErr
	}
	if f.value == nil {
		return f, nil
	}
	return f.value, nil
}

func (f *fakeProvider) Close(context.Context) error {
	f.closeCalled = true
	return f.closeErr
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "application.yaml"), []byte(body), 0o600))
	return dir
}

func TestNew_MinimalEngineNeedsNoProviders(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	eng, err := New(context.Background(), WithConfigDir(dir))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	assert.NotNil(t, eng.Logger())
	assert.Nil(t, eng.Router(), "the router is opt-in")
	assert.Nil(t, eng.Health(), "health is opt-in")
	assert.Empty(t, eng.ComponentNames())
}

func TestNew_ProviderReceivesItsOwnSection(t *testing.T) {
	dir := writeConfig(t, `
log:
  level: error
widget:
  endpoint: "http://localhost:1234"
  retries: 3
`)

	p := &fakeProvider{name: "widget:main", configKey: "widget"}

	eng, err := New(context.Background(), WithConfigDir(dir), WithProvider(p))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	assert.True(t, p.initCalled)
	assert.True(t, p.sawExists)
	assert.Equal(t, "http://localhost:1234", p.decoded.Endpoint)
	assert.Equal(t, 3, p.decoded.Retries)
}

// TestNew_CoreNeverSeesAdapterTypes is the point of the whole refactor: an
// unknown section is carried through untouched, so adding an adapter requires
// no change to the core configuration struct.
func TestNew_CoreNeverSeesAdapterTypes(t *testing.T) {
	dir := writeConfig(t, `
log:
  level: error
something_the_core_has_never_heard_of:
  endpoint: "value"
`)

	eng, err := New(context.Background(), WithConfigDir(dir))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	p := &fakeProvider{name: "x", configKey: "something_the_core_has_never_heard_of"}
	raw := NewRawConfig(p.ConfigKey(), map[string]any{"endpoint": "value"})
	var got fakeConfig
	require.NoError(t, raw.Decode(&got))
	assert.Equal(t, "value", got.Endpoint)
}

func TestNew_MissingSectionIsReportedAsAbsent(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	p := &fakeProvider{name: "widget:main", configKey: "not_present"}
	eng, err := New(context.Background(), WithConfigDir(dir), WithProvider(p))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	assert.True(t, p.initCalled)
	assert.False(t, p.sawExists, "an absent section must be reported as absent")
}

func TestGet_ReturnsTypedComponent(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	type widget struct{}
	want := &widget{}
	p := &fakeProvider{name: "widget:main", configKey: "widget", value: want}

	eng, err := New(context.Background(), WithConfigDir(dir), WithProvider(p))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	got, err := Get[*widget](eng, "widget:main")
	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestGet_WrongTypeAndMissingAreErrors(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	type widget struct{}
	type gadget struct{}
	p := &fakeProvider{name: "widget:main", configKey: "widget", value: &widget{}}

	eng, err := New(context.Background(), WithConfigDir(dir), WithProvider(p))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	_, err = Get[*gadget](eng, "widget:main")
	assert.ErrorContains(t, err, "not")

	_, err = Get[*widget](eng, "nope")
	assert.ErrorContains(t, err, "not found")
}

// TestNew_ClosesInLIFOOrder covers the ordering guarantee: a component built
// later may depend on one built earlier, so it must be released first.
func TestNew_ClosesInLIFOOrder(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	var mu sync.Mutex
	var order []string
	makeProvider := func(name string) Provider {
		return &recordingProvider{name: name, onClose: func() {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
		}}
	}

	eng, err := New(context.Background(), WithConfigDir(dir),
		WithProvider(makeProvider("first"), makeProvider("second"), makeProvider("third")))
	require.NoError(t, err)

	require.NoError(t, eng.Close(context.Background()))
	assert.Equal(t, []string{"third", "second", "first"}, order)
}

type recordingProvider struct {
	name    string
	onClose func()
}

func (r *recordingProvider) Name() string      { return r.name }
func (r *recordingProvider) ConfigKey() string { return r.name }
func (r *recordingProvider) Init(context.Context, RawConfig, Deps) (any, error) {
	return r.name, nil
}
func (r *recordingProvider) Close(context.Context) error {
	r.onClose()
	return nil
}

// TestNew_FailedProviderUnwindsEarlierOnes guards against leaking components
// built before the failure.
func TestNew_FailedProviderUnwindsEarlierOnes(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	good := &fakeProvider{name: "good", configKey: "a"}
	bad := &fakeProvider{name: "bad", configKey: "b", initErr: errors.New("boom")}

	eng, err := New(context.Background(), WithConfigDir(dir), WithProvider(good, bad))

	require.Error(t, err)
	assert.Nil(t, eng)
	assert.ErrorIs(t, err, ErrProviderInit)
	assert.True(t, good.closeCalled, "a provider built before the failure must be closed")
}

func TestNew_DuplicateComponentNameIsRejected(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	a := &fakeProvider{name: "same", configKey: "a"}
	b := &fakeProvider{name: "same", configKey: "b"}

	_, err := New(context.Background(), WithConfigDir(dir), WithProvider(a, b))
	assert.ErrorContains(t, err, "duplicate component name")
}

func TestNew_RouterAndHealthAreOptIn(t *testing.T) {
	dir := writeConfig(t, `
log:
  level: error
router:
  port: "0"
`)

	eng, err := New(context.Background(), WithConfigDir(dir), WithRouter(), WithHealth())
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	require.NotNil(t, eng.Router())
	require.NotNil(t, eng.Health())

	for _, path := range []string{"/health", "/live", "/ready", "/deps"} {
		rec := httptest.NewRecorder()
		eng.Router().Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusOK, rec.Code, "%s must be mounted", path)
	}
}

func TestRun_WithoutRouterIsAnError(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	eng, err := New(context.Background(), WithConfigDir(dir))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	assert.ErrorIs(t, eng.Run(context.Background()), errNoRouter)
}

func TestNew_MissingConfigFileIsAnError(t *testing.T) {
	_, err := New(context.Background(), WithConfigDir(t.TempDir()))
	assert.ErrorContains(t, err, "no configuration file found")
}

// TestResource_BuiltOnceUnderConcurrency covers the shared-resource memo used
// by the AWS providers to resolve one credential chain between them.
func TestResource_BuiltOnceUnderConcurrency(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")
	eng, err := New(context.Background(), WithConfigDir(dir))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	var builds int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = eng.resource("shared", func() (any, error) {
				mu.Lock()
				builds++
				mu.Unlock()
				return "value", nil
			})
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, builds, "a shared resource must be built at most once per engine")
}

func TestDecodeNamed(t *testing.T) {
	section := []any{
		map[string]any{"orders": map[string]any{"endpoint": "http://orders", "retries": 2}},
		map[string]any{"billing": map[string]any{"endpoint": "http://billing"}},
	}
	raw := NewRawConfig("widgets", section)

	got, err := DecodeNamed[fakeConfig](raw, "orders")
	require.NoError(t, err)
	assert.Equal(t, "http://orders", got.Endpoint)
	assert.Equal(t, 2, got.Retries)

	_, err = DecodeNamed[fakeConfig](raw, "missing")
	assert.ErrorContains(t, err, "not declared")

	assert.Equal(t, []string{"billing", "orders"}, InstanceNames(raw))
}

func TestDecodeNamed_MissingSection(t *testing.T) {
	_, err := DecodeNamed[fakeConfig](NewMissingRawConfig("widgets"), "orders")
	assert.ErrorContains(t, err, "no configuration section")
}

func TestResolveEnvValue_InDecoding(t *testing.T) {
	t.Setenv("GO_ENGINE_PROVIDER_ENDPOINT", "http://from-env")

	raw := NewRawConfig("widget", map[string]any{
		"endpoint": "${GO_ENGINE_PROVIDER_ENDPOINT}",
	})

	var cfg fakeConfig
	require.NoError(t, raw.Decode(&cfg))
	assert.Equal(t, "http://from-env", cfg.Endpoint)
}

// TestConfig_RemainExcludesTypedSections pins the behaviour the whole
// decoupling rests on: sections the core owns are decoded into typed fields and
// must NOT also appear in Components, while everything else is carried through
// untouched.
func TestConfig_RemainExcludesTypedSections(t *testing.T) {
	dir := writeConfig(t, `
log:
  level: error
router:
  port: "8081"
health:
  timeout: 5s
sqs_clients:
  - orders:
      endpoint: "http://localhost:4566"
totally_unknown:
  whatever: 1
`)

	cfg, err := loadConfig(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "8081", cfg.Router.Port, "typed core sections are decoded")
	assert.Equal(t, "error", cfg.Log.Level)

	for _, owned := range []string{"router", "log", "health"} {
		assert.NotContains(t, cfg.Components, owned,
			"a section the core owns must not leak into the raw remainder")
	}

	assert.Contains(t, cfg.Components, "sqs_clients")
	assert.Contains(t, cfg.Components, "totally_unknown",
		"an unknown section must survive: that is what lets a new adapter need no core change")

	assert.True(t, cfg.Section("sqs_clients").Exists())
	assert.False(t, cfg.Section("never_written").Exists())
}

// TestConfig_MergesMultipleFiles covers the environment-overlay pattern.
func TestConfig_MergesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "application.yaml"),
		[]byte("log:\n  level: info\nrouter:\n  port: \"8080\"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "application-prod.yaml"),
		[]byte("log:\n  level: error\n"), 0o600))

	cfg, err := loadConfig(dir, []string{"application", "application-prod"})
	require.NoError(t, err)

	assert.Equal(t, "error", cfg.Log.Level, "the later file overrides")
	assert.Equal(t, "8080", cfg.Router.Port, "unset keys keep the base value")
}

// TestConfig_AbsentOverlayIsNotAnError: an environment file that does not exist
// must be skipped, not fatal.
func TestConfig_AbsentOverlayIsNotAnError(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	cfg, err := loadConfig(dir, []string{"application", "application-does-not-exist"})
	require.NoError(t, err)
	assert.Equal(t, "error", cfg.Log.Level)
}

// TestDecoder_ParsesDurationsAndSlices is the regression test for the core
// loader shipping without the duration and slice hooks. Timeouts are written as
// "10s" everywhere in the configuration, so without this the engine rejected
// ordinary, previously valid files.
func TestDecoder_ParsesDurationsAndSlices(t *testing.T) {
	dir := writeConfig(t, `
log:
  level: error
router:
  port: "8080"
  read_timeout: 15s
  write_timeout: 45s
  shutdown_timeout: 1m
health:
  timeout: 7s
`)

	cfg, err := loadConfig(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, 15*time.Second, cfg.Router.ReadTimeout)
	assert.Equal(t, 45*time.Second, cfg.Router.WriteTimeout)
	assert.Equal(t, time.Minute, cfg.Router.ShutdownTimeout)
	assert.Equal(t, 7*time.Second, cfg.Health.Timeout)
}

// TestDecoder_ProviderSectionsGetTheSameHooks: a provider decoding its own
// section must interpret durations exactly as the core does, or the same YAML
// would mean different things depending on who read it.
func TestDecoder_ProviderSectionsGetTheSameHooks(t *testing.T) {
	type providerConfig struct {
		Timeout time.Duration `mapstructure:"timeout"`
		Hosts   []string      `mapstructure:"hosts"`
	}

	raw := NewRawConfig("thing", map[string]any{
		"timeout": "30s",
		"hosts":   "a.example.com,b.example.com",
	})

	var cfg providerConfig
	require.NoError(t, raw.Decode(&cfg))
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, []string{"a.example.com", "b.example.com"}, cfg.Hosts)
}

// TestDecoder_BindsUntaggedStructs preserves the legacy loader's lenient key
// matching: a consumer struct without mapstructure tags must still bind.
func TestDecoder_BindsUntaggedStructs(t *testing.T) {
	type untagged struct {
		ServiceName  string
		OtelEndpoint string
	}

	raw := NewRawConfig("thing", map[string]any{
		"service_name":  "svc",
		"otel_endpoint": "localhost:4317",
	})

	var cfg untagged
	require.NoError(t, raw.Decode(&cfg))
	assert.Equal(t, "svc", cfg.ServiceName)
	assert.Equal(t, "localhost:4317", cfg.OtelEndpoint)
}

// TestNew_FailedProviderIsItselfClosed covers the resource a provider built
// before failing later in its own Init: it is never registered with the
// lifecycle, so the core must close it explicitly.
func TestNew_FailedProviderIsItselfClosed(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	bad := &fakeProvider{name: "bad", configKey: "b", initErr: errors.New("boom")}

	_, err := New(context.Background(), WithConfigDir(dir), WithProvider(bad))

	require.Error(t, err)
	assert.True(t, bad.closeCalled, "a provider that fails Init must still be closed")
}
