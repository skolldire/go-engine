package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/skolldire/go-engine/pkg/utilities/app_profile"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

var _ Service = (*App)(nil)

func WithLogger(logger logger.Service) RouterOption {
	return func(a *App) {
		a.logger = logger
	}
}

func NewService(c Config, opts ...RouterOption) *App {
	if c.ReadTimeout == 0 {
		c.ReadTimeout = defaultReadTimeout
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = defaultWriteTimeout
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = defaultShutdownTimeout
	}
	if c.HandlerTimeout == 0 {
		// Track WriteTimeout so the two stay coherent when only one is set.
		c.HandlerTimeout = c.WriteTimeout
		if c.HandlerTimeout == 0 {
			c.HandlerTimeout = defaultHandlerTimeout
		}
	}

	app := &App{
		router:          chi.NewRouter(),
		config:          c,
		shutdownTimeout: c.ShutdownTimeout,
	}

	for _, opt := range opts {
		opt(app)
	}

	// NewService(cfg) without WithLogger is a supported call; without this the
	// first a.logger.Info in Run panicked on a nil interface.
	if app.logger == nil {
		app.logger = noopLogger{}
	}

	app.configureMiddlewares()

	app.configureBasicRoutes()

	app.server = &http.Server{
		Addr:         ":" + setPort(c.Port),
		Handler:      app.router,
		ReadTimeout:  c.ReadTimeout,
		WriteTimeout: c.WriteTimeout,
		IdleTimeout:  c.IdleTimeout,
	}

	return app
}

func (a *App) configureMiddlewares() {
	a.router.Use(middleware.RequestID)
	// Trust X-Forwarded-For / X-Real-IP only when the request arrives through a
	// configured trusted proxy; otherwise keep the direct peer address. When no
	// trusted proxies are configured the headers are ignored entirely, which is
	// the safe default against client IP spoofing.
	if len(a.config.TrustedProxies) > 0 {
		a.router.Use(trustedRealIP(newTrustedProxySet(a.config.TrustedProxies)))
	}
	a.router.Use(middleware.Logger)
	a.router.Use(middleware.Recoverer)
	a.router.Use(middleware.Timeout(a.config.HandlerTimeout))
	a.router.Use(middleware.Compress(5))
	if a.config.EnableCORS {
		a.router.Use(cors.Handler(cors.Options{
			AllowedOrigins:   a.config.CorsConfig.AllowOrigins,
			AllowedMethods:   a.config.CorsConfig.AllowMethods,
			AllowedHeaders:   a.config.CorsConfig.AllowHeaders,
			ExposedHeaders:   a.config.CorsConfig.ExposedHeaders,
			AllowCredentials: a.config.CorsConfig.AllowCredentials,
			MaxAge:           a.config.CorsConfig.AllowMaxAge,
		}))
	}
}

func (a *App) configureBasicRoutes() {
	a.router.Get("/ping", pingHandler)
	// pprof is opt-in and never registered under the production profile.
	if a.config.EnablePprof && !app_profile.IsProdProfile() {
		registerPprofRoutes(a.router)
	}
}

func (a *App) WithMiddleware(middleware func(http.Handler) http.Handler) RouterOption {
	return func(a *App) {
		a.router.Use(middleware)
	}
}

func (a *App) Mount(pattern string, handler http.Handler) {
	a.router.Mount(pattern, handler)
}

func (a *App) Use(middlewares ...func(http.Handler) http.Handler) {
	a.router.Use(middlewares...)
}

func (a *App) HandleFunc(pattern string, handlerFn http.HandlerFunc) {
	a.router.HandleFunc(pattern, handlerFn)
}

func (a *App) AddRoute(method, pattern string, handler http.HandlerFunc) {
	a.router.Method(method, pattern, handler)
}

func (a *App) Router() *chi.Mux {
	return a.router
}

// RegisterShutdownHook registers fn to be called during graceful shutdown,
// after the HTTP server stops accepting connections. Safe for concurrent use:
// nothing stops a caller from registering a hook after Run has started.
func (a *App) RegisterShutdownHook(fn func(context.Context) error) {
	if fn == nil {
		return
	}
	a.hooksMu.Lock()
	defer a.hooksMu.Unlock()
	a.shutdownHooks = append(a.shutdownHooks, fn)
}

// takeShutdownHooks returns the registered hooks and clears the list, so hooks
// run exactly once and a concurrent registration cannot mutate the slice while
// it is being iterated.
func (a *App) takeShutdownHooks() []func(context.Context) error {
	a.hooksMu.Lock()
	defer a.hooksMu.Unlock()
	hooks := a.shutdownHooks
	a.shutdownHooks = nil
	return hooks
}

// Run starts the HTTP server and blocks until one of the following happens:
//   - ctx is cancelled (e.g. the builder context or a parent shutdown),
//   - an OS interrupt/SIGTERM is received, or
//   - the server fails to start.
//
// On any of the first two, it performs a graceful shutdown bounded by
// ShutdownTimeout and runs the registered shutdown hooks. Signal handling is
// released with signal.Stop before returning.
func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	errorCh := make(chan error, 1)

	go func() {
		a.logger.Info(ctx, "starting server", map[string]any{
			"address": a.server.Addr,
		})
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorCh <- err
		}
	}()

	var startErr error
	select {
	case <-ctx.Done():
		a.logger.Info(ctx, "context cancelled, initiating graceful shutdown", nil)
	case <-stop:
		a.logger.Info(ctx, "shutdown signal received, initiating graceful shutdown", nil)
	case startErr = <-errorCh:
		a.logger.Error(ctx, startErr, map[string]any{
			"message": "error starting server",
		})
	}

	// Derive the shutdown context from Background, not ctx: if ctx is already
	// cancelled, the graceful-shutdown deadline must still apply.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()

	// Shutdown and the hooks run on every exit path, including a failed start
	// and a failed shutdown. Returning early on either used to skip the hooks
	// entirely, and Engine.Close is registered as one — so the resources leaked
	// precisely when something had already gone wrong.
	var errs []error
	if startErr != nil {
		errs = append(errs, startErr)
	}

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.Error(ctx, err, map[string]any{
			"message": "error during server shutdown",
		})
		errs = append(errs, fmt.Errorf("server shutdown: %w", err))
	}

	for _, hook := range a.takeShutdownHooks() {
		if err := hook(shutdownCtx); err != nil {
			a.logger.Error(ctx, err, map[string]any{
				"message": "error during shutdown hook",
			})
			errs = append(errs, fmt.Errorf("shutdown hook: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	a.logger.Info(ctx, "server shut down successfully", nil)
	return nil
}

func registerPprofRoutes(router chi.Router) {
	router.Route("/debug/pprof", func(r chi.Router) {
		r.Get("/", pprof.Index)
		r.Get("/cmdline", pprof.Cmdline)
		r.Get("/profile", pprof.Profile)
		r.Get("/symbol", pprof.Symbol)
		r.Get("/trace", pprof.Trace)
		r.Get("/goroutine", pprof.Handler("goroutine").ServeHTTP)
		r.Get("/heap", pprof.Handler("heap").ServeHTTP)
		r.Get("/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
		r.Get("/block", pprof.Handler("block").ServeHTTP)
	})
}

func setPort(p string) string {
	if p != "" {
		return p
	}
	if envPort := os.Getenv("PORT"); envPort != "" {
		return envPort
	}
	return appDefaultPort
}
