package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/skolldire/go-engine/pkg/health"
	"github.com/skolldire/go-engine/pkg/router"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

var (
	errNoRouter = errors.New("engine has no router: build it with engine.WithRouter()")

	// ErrProviderInit wraps any failure raised by a provider's Init.
	ErrProviderInit = errors.New("provider initialization failed")
)

// New assembles an engine.
//
// Options are applied in dependency order rather than call order: configuration
// first, then the logger, then core services, then providers. The old builder
// required callers to know that WithJWTAuth had to follow WithRouter and that
// WithHealth had to follow the config step; here that knowledge lives in one
// place and misordering is impossible.
//
// If any step fails, every component already built is closed before the error
// is returned, so a failed New leaks nothing.
func New(ctx context.Context, opts ...Option) (*Engine, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	b := &builder{}
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}

	cfg, err := b.resolveConfig()
	if err != nil {
		return nil, err
	}

	log := b.resolveLogger(cfg)
	tel := newTelemetrySwitch(b.telemetry)

	eng := &Engine{
		ctx:        ctx,
		log:        log,
		cfg:        cfg,
		components: make(map[string]any, len(b.providers)),
		resources:  make(map[string]*resourceEntry),
		core:       &coreServices{},
		lifecycle:  &lifecycle{},
	}

	b.buildCoreServices(eng, cfg, log)

	if err := b.buildProviders(ctx, eng, cfg, tel, log); err != nil {
		return nil, err
	}
	eng.telemetry = tel

	// Release everything when the HTTP server stops.
	if eng.core.router != nil {
		eng.core.router.RegisterShutdownHook(eng.Close)
	}

	return eng, nil
}

// resolveConfig loads the configuration unless one was supplied outright.
func (b *builder) resolveConfig() (*Config, error) {
	if b.config != nil {
		return b.config, nil
	}
	return loadConfig(b.configDir, b.configFiles)
}

// resolveLogger returns the caller's logger, or one built from the `log:`
// section.
func (b *builder) resolveLogger(cfg *Config) logger.Service {
	if b.logger != nil {
		return b.logger
	}
	return logger.NewService(cfg.Log, nil)
}

// buildCoreServices creates the router and health service the engine owns.
func (b *builder) buildCoreServices(eng *Engine, cfg *Config, log logger.Service) {
	if b.wantHealth {
		hc := cfg.Health
		if b.healthCfg != nil {
			hc = *b.healthCfg
		}
		eng.core.health = health.NewService(hc, log)
	}

	if !b.wantRouter {
		return
	}

	eng.core.router = router.NewService(cfg.Router, router.WithLogger(log))
	for _, mw := range b.middlewares {
		mw(eng.core.router)
	}
	mountHealthRoutes(eng)
}

// buildProviders initialises every provider, in registration order.
//
// Each one that succeeds is registered with the lifecycle immediately, so a
// later failure still unwinds it.
func (b *builder) buildProviders(
	ctx context.Context,
	eng *Engine,
	cfg *Config,
	tel *telemetrySwitch,
	log logger.Service,
) error {
	deps := Deps{
		Logger:    log,
		Telemetry: tel,
		Health:    healthRegistrar(eng),
		Section:   cfg.Section,
		Resource:  eng.resource,
	}

	providers := b.providers
	for _, fn := range b.providerFuncs {
		discovered, err := fn(cfg)
		if err != nil {
			return failNew(ctx, eng, fmt.Errorf("discovering providers: %w", err))
		}
		providers = append(providers, discovered...)
	}

	for _, p := range providers {
		if p == nil {
			continue
		}

		// Validate the shape before touching the provider: calling Name on a
		// nil pointer inside a non-nil interface panics.
		if err := requirePointerProvider(p); err != nil {
			return failNew(ctx, eng, err)
		}

		name := p.Name()

		if _, exists := eng.Component(name); exists {
			return failNew(ctx, eng, fmt.Errorf("duplicate component name %q", name))
		}

		// A Provider holds the component it built, so handing the same instance
		// to two engines would have the second Init overwrite the first's
		// client — and the first engine's Close would then release a resource
		// it no longer owns.
		if err := claimProvider(p, name); err != nil {
			return failNew(ctx, eng, err)
		}
		eng.providers = append(eng.providers, p)

		component, err := p.Init(ctx, cfg.Section(p.ConfigKey()), deps)
		if err != nil {
			// Close the provider that failed too: Init may have built several
			// resources and failed on the last one, and it is never reached by
			// the lifecycle because it was never registered.
			closeErr := p.Close(ctx)
			cause := fmt.Errorf("%w: %s: %w", ErrProviderInit, name, err)
			if closeErr != nil {
				cause = errors.Join(cause, fmt.Errorf("closing failed provider %s: %w", name, closeErr))
			}
			return failNew(ctx, eng, cause)
		}

		eng.setComponent(name, component)
		eng.lifecycle.register(name, p.Close)

		// A provider that supplies telemetry replaces the no-op for everyone,
		// including the providers already built.
		if source, ok := p.(TelemetrySource); ok {
			if impl := source.Telemetry(); impl != nil {
				tel.set(impl)
			}
		}
	}

	return nil
}

// failNew unwinds a partially built engine and returns the original error,
// joined with any rollback failure so nothing is swallowed.
func failNew(ctx context.Context, eng *Engine, cause error) error {
	if err := eng.Close(ctx); err != nil {
		return errors.Join(cause, fmt.Errorf("rollback: %w", err))
	}
	return cause
}

// mountHealthRoutes exposes the probe endpoints when both a router and a health
// service exist.
func mountHealthRoutes(e *Engine) {
	if e.core.router == nil || e.core.health == nil {
		return
	}
	h := health.NewHTTPHandler(e.core.health)
	e.core.router.AddRoute("GET", "/health", h.HealthHandler)
	e.core.router.AddRoute("GET", "/live", h.LiveHandler)
	e.core.router.AddRoute("GET", "/ready", h.ReadyHandler)
	e.core.router.AddRoute("GET", "/deps", h.DepsHandler)
}

// healthRegistrar adapts the optional health service to the provider-facing
// interface, degrading to a no-op when health was not enabled.
func healthRegistrar(e *Engine) HealthRegistrar {
	if e.core.health == nil {
		return noopHealth{}
	}
	return &healthServiceRegistrar{svc: e.core.health}
}

type healthServiceRegistrar struct {
	svc *health.HealthService
}

func (h *healthServiceRegistrar) RegisterCheck(name string, check func(ctx context.Context) error) {
	if check == nil {
		return
	}
	h.svc.Register(name, health.CheckerFunc(check))
}
