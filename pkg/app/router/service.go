package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	app := &App{
		router:          chi.NewRouter(),
		config:          c,
		shutdownTimeout: c.ShutdownTimeout,
	}

	for _, opt := range opts {
		opt(app)
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
	a.router.Use(middleware.Timeout(60 * time.Second))
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
// after the HTTP server stops accepting connections.
func (a *App) RegisterShutdownHook(fn func(context.Context) error) {
	a.shutdownHooks = append(a.shutdownHooks, fn)
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
		a.logger.Info(ctx, "Iniciando servidor", map[string]interface{}{
			"address": a.server.Addr,
		})
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		a.logger.Info(ctx, "context cancelled, initiating graceful shutdown", nil)
	case <-stop:
		a.logger.Info(ctx, "shutdown signal received, initiating graceful shutdown", nil)
	case err := <-errorCh:
		a.logger.Error(ctx, err, map[string]interface{}{
			"message": "error starting server",
		})
		return err
	}

	// Derive the shutdown context from Background, not ctx: if ctx is already
	// cancelled, the graceful-shutdown deadline must still apply.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.Error(ctx, err, map[string]interface{}{
			"message": "error during server shutdown",
		})
		return err
	}

	for _, hook := range a.shutdownHooks {
		if err := hook(shutdownCtx); err != nil {
			a.logger.Error(ctx, err, map[string]interface{}{
				"message": "error during shutdown hook",
			})
		}
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
