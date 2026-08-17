package engine

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"

	"github.com/skolldire/go-engine/pkg/health"
	"github.com/skolldire/go-engine/pkg/router"
)

// coreServices holds the services the core itself can build. Each is opt-in:
// an engine with none of them is valid and pulls in nothing extra at runtime.
type coreServices struct {
	router router.Service
	health *health.HealthService
}

// Option configures an engine during New. Options are declarative: New applies
// them in a fixed, dependency-correct order regardless of the order the caller
// wrote them, so there is no "call X before Y" rule to remember.
type Option func(*builder)

type builder struct {
	configDir   string
	configFiles []string
	config      *Config

	logger    logger.Service
	telemetry Telemetry

	wantRouter bool
	wantHealth bool
	healthCfg  *health.Config

	providers     []Provider
	providerFuncs []func(*Config) ([]Provider, error)
	middlewares   []func(router.Service)
}

// Note: there is deliberately no WithConfigWatch option. Live reload requires
// republishing the new configuration to components that were already built,
// which the Provider contract does not yet express. An option that starts a
// watcher nobody listens to is the defect the audit found in the old
// WithDynamicConfig, so it is left unimplemented rather than half-implemented.

// WithConfigDir reads configuration from dir instead of CONF_DIR or ./config.
func WithConfigDir(dir string) Option {
	return func(b *builder) { b.configDir = dir }
}

// WithConfigFiles selects which file base names to merge, in order. Defaults to
// {"application"}. Names are given without extension.
func WithConfigFiles(names ...string) Option {
	return func(b *builder) { b.configFiles = names }
}

// WithConfig supplies an already-built configuration and skips file loading.
// Primarily for tests and for applications that source configuration elsewhere.
func WithConfig(cfg *Config) Option {
	return func(b *builder) { b.config = cfg }
}

// WithLogger overrides the logger built from the `log:` configuration section.
func WithLogger(l logger.Service) Option {
	return func(b *builder) { b.logger = l }
}

// WithTelemetry installs a telemetry implementation.
//
// The core defines only the Telemetry interface; the OpenTelemetry
// implementation lives in its own package so that an application without
// telemetry never links the OTel SDK.
func WithTelemetry(t Telemetry) Option {
	return func(b *builder) { b.telemetry = t }
}

// WithRouter builds the HTTP router from the `router:` section.
func WithRouter() Option {
	return func(b *builder) { b.wantRouter = true }
}

// WithMiddleware registers a router middleware. Implies WithRouter.
func WithMiddleware(fn func(router.Service)) Option {
	return func(b *builder) {
		b.wantRouter = true
		if fn != nil {
			b.middlewares = append(b.middlewares, fn)
		}
	}
}

// WithHealth enables the health service and, when a router is present, mounts
// /health, /live, /ready and /deps.
func WithHealth() Option {
	return func(b *builder) { b.wantHealth = true }
}

// WithHealthConfig is WithHealth with explicit settings.
func WithHealthConfig(cfg health.Config) Option {
	return func(b *builder) {
		b.wantHealth = true
		b.healthCfg = &cfg
	}
}

// WithProvider registers a component. This is the only way an adapter enters an
// engine, and the reason the core needs no knowledge of any adapter.
func WithProvider(providers ...Provider) Option {
	return func(b *builder) {
		for _, p := range providers {
			if p != nil {
				b.providers = append(b.providers, p)
			}
		}
	}
}

// WithProviderFunc registers providers discovered from the loaded
// configuration. It is how a preset turns "every SQS queue declared in the
// YAML" into providers without the application naming each one, and without
// the configuration being parsed twice.
//
// The function returns an error so a malformed section fails the build instead
// of silently contributing no providers.
func WithProviderFunc(fn func(*Config) ([]Provider, error)) Option {
	return func(b *builder) {
		if fn != nil {
			b.providerFuncs = append(b.providerFuncs, fn)
		}
	}
}

// Router returns the HTTP router, or nil when the engine was built without
// WithRouter.
func (e *Engine) Router() router.Service {
	if e.core == nil {
		return nil
	}
	return e.core.router
}

// Health returns the health service, or nil when the engine was built without
// WithHealth.
func (e *Engine) Health() *health.HealthService {
	if e.core == nil {
		return nil
	}
	return e.core.health
}

// Run starts the HTTP server and blocks until the context is cancelled or a
// shutdown signal arrives. It returns an error when the engine has no router.
func (e *Engine) Run(ctx context.Context) error {
	r := e.Router()
	if r == nil {
		return errNoRouter
	}
	if ctx == nil {
		ctx = e.ctx
	}
	return r.Run(ctx)
}
