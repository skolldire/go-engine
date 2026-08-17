package app

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/config/dynamic"
	"github.com/skolldire/go-engine/pkg/config/viper" //nolint:staticcheck // SA1019: pkg/app is itself deprecated and is the last consumer of this loader
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/health"
	pkgotel "github.com/skolldire/go-engine/pkg/telemetry/otel"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type AppBuilder struct {
	engine        *Engine
	errors        []error
	shutdownHooks []func(context.Context) error
	healthMounted bool
}

func NewAppBuilder() *AppBuilder {
	return &AppBuilder{
		engine: &Engine{
			errors: []error{},
			ctx:    context.Background(),
		},
		errors: []error{},
	}
}

func (b *AppBuilder) WithContext(ctx context.Context) *AppBuilder {
	if ctx == nil {
		b.addError(fmt.Errorf("context cannot be nil"))
		return b
	}
	b.engine.ctx = ctx
	return b
}

func (b *AppBuilder) WithConfigs() *AppBuilder {
	app := &App{Engine: b.engine}
	result := app.GetConfigs()
	b.engine = result.Engine
	b.errors = append(b.errors, b.engine.errors...)
	return b
}

func (b *AppBuilder) WithDynamicConfig() *AppBuilder {
	cfgLogger := newDefaultLogger()

	v := viper.NewService(cfgLogger)

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	tempLog := logger.NewService(logger.Config{
		Level: logLevel,
	}, cfgLogger)

	dynamicConfig, err := v.ApplyDynamic(tempLog)
	if err != nil {
		b.addError(err)
		return b
	}

	config, err := client.SafeTypeAssert[*viper.Config](dynamicConfig.Get())
	if err != nil {
		b.addError(fmt.Errorf("failed to get dynamic config: %w", err))
		return b
	}
	b.engine.Conf = config
	b.engine.currentConf.Store(config)
	b.engine.dynamicConfig = dynamicConfig

	log := buildLogger(config.Log, cfgLogger)
	b.engine.Log = log

	// Populate the flags here too: previously only WithConfigs+WithInitialization
	// did it, so WithDynamicConfig alone left GetFeatureFlags returning nil.
	// Always construct it so the reload hook has a stable instance to update.
	b.engine.FeatureFlags = dynamic.NewFeatureFlags(config.FeatureFlags, log)

	// Publish every successful reload so Config() actually returns new values.
	// Without this hook the watcher updated its own copy and no consumer ever
	// observed the change.
	dynamicConfig.AddReloadHook(func(_, newConfig any) error {
		reloaded, err := client.SafeTypeAssert[*viper.Config](newConfig)
		if err != nil {
			return fmt.Errorf("reloaded config has unexpected type: %w", err)
		}

		b.engine.currentConf.Store(reloaded)
		// Update in place rather than swapping the pointer: this hook runs on the
		// watcher goroutine, and SetAll is the only race-free way to publish to
		// consumers that already hold the *FeatureFlags.
		if ff := b.engine.FeatureFlags; ff != nil {
			ff.SetAll(reloaded.FeatureFlags)
		}
		return nil
	})

	ctx := b.engine.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := dynamicConfig.StartWatching(ctx); err != nil {
		b.engine.Log.Warn(ctx, "failed to start config watchers: "+err.Error(), nil)
	}

	// The watcher owns goroutines and fsnotify handles. Registering Stop is what
	// makes them reachable at shutdown; previously the only reference was local
	// to this function and the goroutines leaked for the process lifetime.
	b.engine.registerSimpleCloser("dynamic-config", dynamicConfig.Stop)

	return b
}

func (b *AppBuilder) WithInitialization() *AppBuilder {
	if len(b.errors) > 0 {
		return b
	}
	app := &App{Engine: b.engine}
	result := app.Init()
	b.engine = result.Engine
	b.errors = append(b.errors, b.engine.errors...)
	return b
}

func (b *AppBuilder) WithRouter() *AppBuilder {
	if len(b.errors) > 0 {
		return b
	}
	app := &App{Engine: b.engine}
	result := app.InitializeRouter()
	b.engine = result.Engine
	b.errors = append(b.errors, b.engine.errors...)

	// Register any shutdown hooks accumulated before router was initialized.
	if b.engine.Router != nil {
		for _, hook := range b.shutdownHooks {
			b.engine.Router.RegisterShutdownHook(hook)
		}
		b.shutdownHooks = nil

		// Release all resources created during initialization (Redis, Mongo,
		// RabbitMQ, gRPC, Kafka, telemetry, …) in LIFO order when the server
		// shuts down. Registered last so it runs after any user hooks.
		b.engine.Router.RegisterShutdownHook(b.engine.Close)
	}
	b.mountHealthIfReady()
	return b
}

func (b *AppBuilder) WithMiddleware(middleware func(router.Service)) *AppBuilder {
	if b.engine.Router == nil {
		b.addError(fmt.Errorf("router not initialized, call WithRouter first"))
		return b
	}
	if middleware != nil {
		middleware(b.engine.Router)
	}
	return b
}

func (b *AppBuilder) WithGracefulShutdown() *AppBuilder {
	return b
}

// WithOTEL initializes the OpenTelemetry provider and wires its Shutdown into
// the router's graceful shutdown sequence. Call after WithDynamicConfig; may be
// called before or after WithRouter.
func (b *AppBuilder) WithOTEL(cfg pkgotel.OTELConfig) *AppBuilder {
	if len(b.errors) > 0 {
		return b
	}
	ctx := b.engine.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	provider, err := pkgotel.NewProvider(ctx, cfg)
	if err != nil {
		b.addError(fmt.Errorf("init otel provider: %w", err))
		return b
	}
	if b.engine.Services == nil {
		b.engine.Services = NewServiceRegistry()
	}
	b.engine.Services.OTELProvider = provider

	if b.engine.Router != nil {
		b.engine.Router.RegisterShutdownHook(provider.Shutdown)
	} else {
		b.shutdownHooks = append(b.shutdownHooks, provider.Shutdown)
	}
	return b
}

func (b *AppBuilder) WithHealth(cfg health.Config) *AppBuilder {
	if len(b.errors) > 0 {
		return b
	}
	if b.engine.Log == nil {
		b.addError(fmt.Errorf("logger not initialized, call WithDynamicConfig first"))
		return b
	}
	if b.engine.Services == nil {
		b.engine.Services = NewServiceRegistry()
	}
	b.engine.Services.Health = health.NewService(cfg, b.engine.Log)
	b.mountHealthIfReady()
	return b
}

// RegisterHealthChecker adds a named checker to the health service.
// If WithHealth has not been called yet, the service is initialized with default config.
// Call after WithDynamicConfig or SetLogger so the logger is available.
func (b *AppBuilder) RegisterHealthChecker(name string, c health.Checker) *AppBuilder {
	if len(b.errors) > 0 {
		return b
	}
	if b.engine.Services == nil {
		b.engine.Services = NewServiceRegistry()
	}
	if b.engine.Services.Health == nil {
		if b.engine.Log == nil {
			b.addError(fmt.Errorf("call WithDynamicConfig or SetLogger before RegisterHealthChecker"))
			return b
		}
		b.engine.Services.Health = health.NewService(health.Config{}, b.engine.Log)
	}
	b.engine.Services.Health.Register(name, c)
	b.mountHealthIfReady()
	return b
}

// mountHealthIfReady registers the health endpoints on the router when both the
// health service and router are available. It is idempotent.
//
// Four routes are mounted:
//
//	GET /health → unified payload for ECS/ALB style checks
//	GET /live   → liveness probe (Kubernetes)
//	GET /ready  → readiness probe (Kubernetes)
//	GET /deps   → per-dependency JSON status
//
// The last three used to be defined by health.HTTPHandler.Routes but never
// mounted, so the probes every Kubernetes deployment expects returned 404
// unless the consumer wired them by hand.
func (b *AppBuilder) mountHealthIfReady() {
	if b.healthMounted {
		return
	}
	if b.engine.Router == nil || b.engine.Services == nil || b.engine.Services.Health == nil {
		return
	}
	h := health.NewHTTPHandler(b.engine.Services.Health)
	b.engine.Router.AddRoute("GET", "/health", h.HealthHandler)
	b.engine.Router.AddRoute("GET", "/live", h.LiveHandler)
	b.engine.Router.AddRoute("GET", "/ready", h.ReadyHandler)
	b.engine.Router.AddRoute("GET", "/deps", h.DepsHandler)
	b.healthMounted = true
}

// WithJWTAuth registers the JWT validation middleware on the router.
// It must be called after WithRouter. The middleware validates Bearer tokens
// on every request not listed in cfg.SkipPaths and injects *router.Claims
// into the context. Use router.ClaimsFromContext in handlers to read them.
//
// For Cognito, set JWKSURL to:
//
//	https://cognito-idp.{region}.amazonaws.com/{pool_id}/.well-known/jwks.json
func (b *AppBuilder) WithJWTAuth(cfg router.JWTAuthConfig) *AppBuilder {
	if len(b.errors) > 0 {
		return b
	}
	if b.engine.Router == nil {
		b.addError(fmt.Errorf("router not initialized, call WithRouter before WithJWTAuth"))
		return b
	}
	b.engine.Router.Use(router.JWTAuth(cfg))
	return b
}

func (b *AppBuilder) WithCustomClient(name string, client any) *AppBuilder {
	if name == "" {
		b.addError(fmt.Errorf("client name cannot be empty"))
		return b
	}
	if client == nil {
		b.addError(fmt.Errorf("client cannot be nil"))
		return b
	}

	// Ensure Services registry is initialized
	if b.engine.Services == nil {
		b.engine.Services = NewServiceRegistry()
	}

	// Store the custom client
	b.engine.Services.CustomClients[name] = client
	return b
}

func (b *AppBuilder) Build() (*Engine, error) {
	if len(b.errors) > 0 {
		return nil, b.failBuild()
	}

	if b.engine.Router == nil {
		b.addError(fmt.Errorf("router not initialized"))
	}

	if len(b.errors) > 0 {
		return nil, b.failBuild()
	}

	return b.engine, nil
}

// failBuild rolls back a partially constructed engine by releasing any
// resources already created, then returns the aggregated build error. The
// rollback error (if any) is joined so nothing is silently swallowed.
func (b *AppBuilder) failBuild() error {
	// errors.Join, not %v: the individual causes must stay reachable through
	// errors.Is/errors.As instead of being flattened into a formatted string.
	buildErr := fmt.Errorf("build errors: %w", errors.Join(b.errors...))
	if b.engine == nil {
		return buildErr
	}
	ctx := b.engine.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if closeErr := b.engine.Close(ctx); closeErr != nil {
		return errors.Join(buildErr, fmt.Errorf("rollback: %w", closeErr))
	}
	return buildErr
}

func (b *AppBuilder) GetErrors() []error {
	return b.errors
}

func (b *AppBuilder) addError(err error) {
	if err != nil {
		b.errors = append(b.errors, err)
	}
}

func (b *AppBuilder) SetLogger(log logger.Service) *AppBuilder {
	if log == nil {
		b.addError(fmt.Errorf("logger cannot be nil"))
		return b
	}
	b.engine.Log = log
	return b
}
