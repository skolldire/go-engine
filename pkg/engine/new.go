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

	// 1. Configuration.
	cfg := b.config
	if cfg == nil {
		loaded, err := loadConfig(b.configDir, b.configFiles)
		if err != nil {
			return nil, err
		}
		cfg = loaded
	}

	// 2. Logger.
	log := b.logger
	if log == nil {
		log = logger.NewService(cfg.Log, nil)
	}

	// 3. Telemetry: a stable indirection, so a telemetry provider built in
	//    step 5 also serves the providers built before it.
	tel := newTelemetrySwitch(b.telemetry)

	eng := &Engine{
		ctx:        ctx,
		log:        log,
		components: make(map[string]any, len(b.providers)),

		resources: make(map[string]*resourceEntry),
		core:      &coreServices{},
		lifecycle: &lifecycle{},
	}

	// 4. Core services.
	svc := log

	if b.wantHealth {
		hc := cfg.Health
		if b.healthCfg != nil {
			hc = *b.healthCfg
		}
		eng.core.health = health.NewService(hc, svc)
	}

	if b.wantRouter {
		eng.core.router = router.NewService(cfg.Router, router.WithLogger(svc))

		for _, mw := range b.middlewares {
			mw(eng.core.router)
		}
		mountHealthRoutes(eng)
	}

	// 5. Providers, in registration order. Each one that succeeds is registered
	//    with the lifecycle immediately, so a later failure still unwinds it.
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
			return nil, failNew(ctx, eng, fmt.Errorf("discovering providers: %w", err))
		}
		providers = append(providers, discovered...)
	}

	for _, p := range providers {
		if p == nil {
			continue
		}
		name := p.Name()

		// A Provider holds the component it built, so handing the same instance
		// to two engines would have the second Init overwrite the first's
		// client — and the first engine's Close would then release a resource
		// it no longer owns. Claiming the provider makes the reuse an explicit
		// error instead of a silent aliasing bug.
		if err := claimProvider(p, name); err != nil {
			return nil, failNew(ctx, eng, err)
		}
		eng.providers = append(eng.providers, p)
		if _, exists := eng.Component(name); exists {
			return nil, failNew(ctx, eng, fmt.Errorf("duplicate component name %q", name))
		}

		component, err := p.Init(ctx, cfg.Section(p.ConfigKey()), deps)
		if err != nil {
			// Close the provider that failed too: Init may have built several
			// resources and failed on the last one, and it is never reached by
			// the lifecycle because it was never registered. Every provider's
			// Close is required to be safe before a successful Init.
			closeErr := p.Close(ctx)
			cause := fmt.Errorf("%w: %s: %w", ErrProviderInit, name, err)
			if closeErr != nil {
				cause = errors.Join(cause, fmt.Errorf("closing failed provider %s: %w", name, closeErr))
			}
			return nil, failNew(ctx, eng, cause)
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

	eng.telemetry = tel

	// 6. Release everything when the HTTP server stops.
	if eng.core.router != nil {
		eng.core.router.RegisterShutdownHook(eng.Close)
	}

	return eng, nil
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
