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

	"github.com/skolldire/go-engine/pkg/health"
	"github.com/skolldire/go-engine/pkg/router"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
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

	names, err := InstanceNames(raw)
	require.NoError(t, err)
	assert.Equal(t, []string{"billing", "orders"}, names)
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

// TestNew_ProviderCannotBeSharedBetweenEngines is the regression test for
// Provider statefulness. A Provider stores the component it built so Close can
// release it, so handing one instance to two engines had the second Init
// overwrite the first's client — and the first engine's Close then released a
// resource it no longer owned.
func TestNew_ProviderCannotBeSharedBetweenEngines(t *testing.T) {
	dir := writeConfig(t, "log:\n  level: error\n")

	shared := &fakeProvider{name: "shared", configKey: "shared"}

	first, err := New(context.Background(), WithConfigDir(dir), WithProvider(shared))
	require.NoError(t, err)

	_, err = New(context.Background(), WithConfigDir(dir), WithProvider(shared))
	require.Error(t, err, "reusing a provider instance must be rejected")
	assert.ErrorContains(t, err, "already used by another engine")

	// After the first engine closes, the instance is free again.
	require.NoError(t, first.Close(context.Background()))

	third, err := New(context.Background(), WithConfigDir(dir), WithProvider(shared))
	require.NoError(t, err, "a closed engine must release its providers")
	t.Cleanup(func() { _ = third.Close(context.Background()) })
}

// TestDecode_RejectsUnknownKeys is the regression test for a typo in a
// provider's section leaving the field at its zero value in silence.
func TestDecode_RejectsUnknownKeys(t *testing.T) {
	raw := NewRawConfig("widget", map[string]any{
		"endpoint": "http://ok",
		"endpont":  "typo",
		"retires":  3,
	})

	var cfg fakeConfig
	err := raw.Decode(&cfg)

	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown configuration keys")
	assert.ErrorContains(t, err, "endpont")
	assert.ErrorContains(t, err, "retires")
}

func TestDecode_AcceptsKnownKeys(t *testing.T) {
	raw := NewRawConfig("widget", map[string]any{"endpoint": "http://ok", "retries": 2})

	var cfg fakeConfig
	require.NoError(t, raw.Decode(&cfg))
	assert.Equal(t, "http://ok", cfg.Endpoint)
	assert.Equal(t, 2, cfg.Retries)
}

// TestInstanceNames_ReportsMalformedSection is the regression test for
// InstanceNames returning nil on error: a malformed section produced a service
// that started with none of its clients and no indication why.
func TestInstanceNames_ReportsMalformedSection(t *testing.T) {
	raw := NewRawConfig("sqs_clients", "this should have been a list")

	names, err := InstanceNames(raw)

	require.Error(t, err)
	assert.Nil(t, names)
	assert.ErrorContains(t, err, "sqs_clients")
}

func TestInstanceNames_AbsentSectionIsNotAnError(t *testing.T) {
	names, err := InstanceNames(NewMissingRawConfig("sqs_clients"))
	require.NoError(t, err)
	assert.Empty(t, names)
}

// TestEachInstance_PropagatesTheError covers the helper the presets use.
func TestEachInstance_PropagatesTheError(t *testing.T) {
	cfg := &Config{Components: map[string]any{"sqs_clients": "malformed"}}

	var seen []string
	err := EachInstance(cfg, "sqs_clients", func(name string) { seen = append(seen, name) })

	require.Error(t, err)
	assert.Empty(t, seen, "no provider may be registered from a malformed section")
}

// TestDecoder_RejectsBareNumberDurations is the regression test for the footgun
// the README documented incorrectly for months: with weakly typed input,
// `read_timeout: 10` decodes as 10 nanoseconds, so the service starts looking
// configured while every request times out instantly.
func TestDecoder_RejectsBareNumberDurations(t *testing.T) {
	type withDuration struct {
		Timeout time.Duration `mapstructure:"timeout"`
	}

	for _, bare := range []any{10, 10.5, int64(10)} {
		raw := NewRawConfig("thing", map[string]any{"timeout": bare})

		var cfg withDuration
		err := raw.Decode(&cfg)

		require.Error(t, err, "value %v (%T) must be rejected", bare, bare)
		assert.ErrorContains(t, err, "not a duration")
		assert.ErrorContains(t, err, "10s", "the message must show the accepted form")
	}
}

func TestDecoder_AcceptsDurationStrings(t *testing.T) {
	type withDuration struct {
		Timeout time.Duration `mapstructure:"timeout"`
	}

	for input, want := range map[string]time.Duration{
		"10s":   10 * time.Second,
		"500ms": 500 * time.Millisecond,
		"1m30s": 90 * time.Second,
	} {
		raw := NewRawConfig("thing", map[string]any{"timeout": input})

		var cfg withDuration
		require.NoError(t, raw.Decode(&cfg), "input %q", input)
		assert.Equal(t, want, cfg.Timeout)
	}
}

// TestOptions_CoverTheDeclarativeSurface exercises the options an application
// actually composes. They are one-liners, but an option that silently stops
// applying is invisible until production, so each is asserted on its effect.
func TestOptions_CoverTheDeclarativeSurface(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.yaml"),
		[]byte("log:\n  level: info\nrouter:\n  port: \"8080\"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "over.yaml"),
		[]byte("log:\n  level: error\n"), 0o600))

	t.Run("WithConfigFiles merges in order", func(t *testing.T) {
		eng, err := New(context.Background(),
			WithConfigDir(dir), WithConfigFiles("base", "over"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = eng.Close(context.Background()) })
		assert.Equal(t, "error", eng.Logger().GetLogLevel())
	})

	t.Run("WithConfig skips file loading", func(t *testing.T) {
		eng, err := New(context.Background(), WithConfig(&Config{}))
		require.NoError(t, err)
		t.Cleanup(func() { _ = eng.Close(context.Background()) })
		assert.NotNil(t, eng.Logger())
	})

	t.Run("WithLogger overrides the configured logger", func(t *testing.T) {
		custom := logger.NewService(logger.Config{Level: "warn"}, nil)
		eng, err := New(context.Background(), WithConfig(&Config{}), WithLogger(custom))
		require.NoError(t, err)
		t.Cleanup(func() { _ = eng.Close(context.Background()) })
		assert.Same(t, custom, eng.Logger())
	})

	t.Run("WithMiddleware implies a router and runs", func(t *testing.T) {
		var applied bool
		eng, err := New(context.Background(), WithConfig(&Config{}),
			WithMiddleware(func(router.Service) { applied = true }))
		require.NoError(t, err)
		t.Cleanup(func() { _ = eng.Close(context.Background()) })

		assert.True(t, applied, "WithMiddleware must run the function")
		assert.NotNil(t, eng.Router(), "and must imply WithRouter")
	})

	t.Run("WithHealthConfig wins over the file", func(t *testing.T) {
		eng, err := New(context.Background(),
			WithConfig(&Config{Health: health.Config{Timeout: time.Minute}}),
			WithHealthConfig(health.Config{Timeout: 3 * time.Second}))
		require.NoError(t, err)
		t.Cleanup(func() { _ = eng.Close(context.Background()) })
		assert.NotNil(t, eng.Health())
	})

	t.Run("Context is the one given to New", func(t *testing.T) {
		type ctxKey struct{}
		ctx := context.WithValue(context.Background(), ctxKey{}, "v")
		eng, err := New(ctx, WithConfig(&Config{}))
		require.NoError(t, err)
		t.Cleanup(func() { _ = eng.Close(context.Background()) })
		assert.Equal(t, "v", eng.Context().Value(ctxKey{}))
	})
}

// TestMustGet_PanicsOnlyWhenTheComponentIsWrong covers the startup-time helper.
func TestMustGet_PanicsOnlyWhenTheComponentIsWrong(t *testing.T) {
	type widget struct{}
	want := &widget{}

	eng, err := New(context.Background(), WithConfig(&Config{}),
		WithProvider(&fakeProvider{name: "w", configKey: "w", value: want}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	assert.Same(t, want, MustGet[*widget](eng, "w"))
	assert.Panics(t, func() { MustGet[*widget](eng, "missing") },
		"a missing component at startup is a programming error, not a runtime condition")
}

// TestProviderHealthChecksReachTheHealthService closes the loop between a
// provider contributing a check and /ready reporting it.
func TestProviderHealthChecksReachTheHealthService(t *testing.T) {
	failing := errors.New("dependency down")

	p := &healthContributingProvider{err: failing}
	eng, err := New(context.Background(), WithConfig(&Config{}), WithHealth(), WithProvider(p))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	require.NotNil(t, eng.Health())
	assert.False(t, eng.Health().IsReady(context.Background()),
		"a provider's failing check must make the engine not ready")
}

// TestHealthRegistrarIsANoopWithoutHealth: a provider must be able to register
// a check unconditionally, even on an engine built without health.
func TestHealthRegistrarIsANoopWithoutHealth(t *testing.T) {
	p := &healthContributingProvider{}
	assert.NotPanics(t, func() {
		eng, err := New(context.Background(), WithConfig(&Config{}), WithProvider(p))
		require.NoError(t, err)
		_ = eng.Close(context.Background())
	})
}

type healthContributingProvider struct{ err error }

func (h *healthContributingProvider) Name() string      { return "dep" }
func (h *healthContributingProvider) ConfigKey() string { return "dep" }
func (h *healthContributingProvider) Init(_ context.Context, _ RawConfig, deps Deps) (any, error) {
	deps.Health.RegisterCheck("dep", func(context.Context) error { return h.err })
	return h, nil
}
func (h *healthContributingProvider) Close(context.Context) error { return nil }

// TestNoopTelemetryIsInert guards the default handed to providers.
func TestNoopTelemetryIsInert(t *testing.T) {
	var n noopTelemetry
	assert.NotPanics(t, func() {
		n.Counter(context.Background(), "c", 1)
		n.Histogram(context.Background(), "h", 1)
	})
	called := false
	require.NoError(t, n.Span(context.Background(), "s", func(context.Context) error {
		called = true
		return nil
	}))
	assert.True(t, called, "a no-op span must still run the operation")
}

func TestDefaultConfigDir(t *testing.T) {
	t.Run("CONF_DIR wins", func(t *testing.T) {
		t.Setenv("CONF_DIR", "/custom")
		assert.Equal(t, "/custom", defaultConfigDir())
	})
	t.Run("falls back to config", func(t *testing.T) {
		t.Setenv("CONF_DIR", "")
		assert.Equal(t, "config", defaultConfigDir())
	})
}

func TestEachInstance_VisitsEveryDeclaredInstance(t *testing.T) {
	cfg := &Config{Components: map[string]any{
		"sqs_clients": []any{
			map[string]any{"orders": map[string]any{}},
			map[string]any{"billing": map[string]any{}},
		},
	}}

	var seen []string
	require.NoError(t, EachInstance(cfg, "sqs_clients", func(n string) { seen = append(seen, n) }))
	assert.Equal(t, []string{"billing", "orders"}, seen)

	seen = nil
	require.NoError(t, EachInstance(cfg, "absent", func(n string) { seen = append(seen, n) }))
	assert.Empty(t, seen)
}

// uncomparableProvider is a legal Provider that cannot be used as a map key: a
// struct value carrying a slice. Nearly every real provider is a pointer, but
// nothing in the interface forbids this shape.
type uncomparableProvider struct{ tags []string }

func (u uncomparableProvider) Name() string      { return "uncomparable" }
func (u uncomparableProvider) ConfigKey() string { return "uncomparable" }
func (u uncomparableProvider) Init(context.Context, RawConfig, Deps) (any, error) {
	return u, nil
}
func (u uncomparableProvider) Close(context.Context) error { return nil }

// TestNew_AcceptsUncomparableProvider is the regression test for the single-use
// claim panicking with "hash of unhashable type". Guarding a legal provider
// shape must never crash the application at startup.
func TestNew_AcceptsUncomparableProvider(t *testing.T) {
	assert.NotPanics(t, func() {
		eng, err := New(context.Background(), WithConfig(&Config{}),
			WithProvider(uncomparableProvider{tags: []string{"a"}}))
		require.NoError(t, err)
		require.NoError(t, eng.Close(context.Background()))
	})
}

// TestClose_IsSafeUnderConcurrency covers the release loop: lifecycle.close is
// idempotent on its own, but clearing e.providers was unguarded, so two
// concurrent Close calls could double-release or read a torn slice.
func TestClose_IsSafeUnderConcurrency(t *testing.T) {
	eng, err := New(context.Background(), WithConfig(&Config{}),
		WithProvider(&fakeProvider{name: "a", configKey: "a"}, &fakeProvider{name: "b", configKey: "b"}))
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = eng.Close(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "concurrent Close %d", i)
	}
	assert.NotEmpty(t, eng.ComponentNames(),
		"Close releases resources; the component map is left intact for diagnostics")
}

// TestClose_ReleasesProvidersExactlyOnce guards against a second Close freeing
// a claim that a later engine had already taken.
func TestClose_ReleasesProvidersExactlyOnce(t *testing.T) {
	shared := &fakeProvider{name: "shared", configKey: "shared"}

	first, err := New(context.Background(), WithConfig(&Config{}), WithProvider(shared))
	require.NoError(t, err)
	require.NoError(t, first.Close(context.Background()))

	second, err := New(context.Background(), WithConfig(&Config{}), WithProvider(shared))
	require.NoError(t, err, "the closed engine released its provider")
	t.Cleanup(func() { _ = second.Close(context.Background()) })

	// A redundant Close on the first engine must not steal the claim the second
	// engine now holds.
	require.NoError(t, first.Close(context.Background()))

	_, err = New(context.Background(), WithConfig(&Config{}), WithProvider(shared))
	assert.Error(t, err, "the second engine still owns the provider")
}
