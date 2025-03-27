package router

import (
	"context"
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
	"github.com/uala-challenge/simple-toolkit/pkg/simplify/simple_router/ping"
)

var _ Service = (*App)(nil)

func WithLogger(logger logger.Service) RouterOption {
	return func(a *App) {
		a.logger = logger
	}
}

func WithMiddleware(middleware func(http.Handler) http.Handler) RouterOption {
	return func(a *App) {
		a.router.Use(middleware)
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
		ReadTimeout:  c.ReadTimeout * time.Second,
		WriteTimeout: c.WriteTimeout * time.Second,
		IdleTimeout:  c.IdleTimeout * time.Second,
	}

	return app
}

func (a *App) configureMiddlewares() {
	a.router.Use(middleware.RequestID)
	a.router.Use(middleware.RealIP)
	a.router.Use(middleware.Logger)
	a.router.Use(middleware.Recoverer)
	a.router.Use(middleware.Timeout(60 * time.Second))
	a.router.Use(middleware.Compress(5))
	if a.config.EnableCORS {
		a.router.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	if len(a.config.TrustedProxies) > 0 {
		for _, proxy := range a.config.TrustedProxies {
			a.router.Use(middleware.SetHeader("X-Forwarded-For", proxy))
		}
	}
}

func (a *App) configureBasicRoutes() {
	a.router.Get("/ping", ping.NewService().Apply())
	if !app_profile.IsProdProfile() {
		registerPprofRoutes(a.router)
	}
}

func (a *App) Mount(pattern string, handler http.Handler) {
	a.router.Mount(pattern, handler)
}

func (a *App) AddRoute(method, pattern string, handler http.HandlerFunc) {
	a.router.Method(method, pattern, handler)
}

func (a *App) Router() *chi.Mux {
	return a.router
}

func (a *App) Run() error {
	ctx := context.Background()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	errorCh := make(chan error, 1)

	go func() {
		a.logger.Info(ctx, "Iniciando servidor", map[string]interface{}{
			"address": a.server.Addr,
		})
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errorCh <- err
		}
	}()

	select {
	case <-stop:
		a.logger.Info(ctx, "Recibida señal de apagado, iniciando shutdown controlado", nil)
	case err := <-errorCh:
		a.logger.Error(ctx, err, map[string]interface{}{
			"message": "Error al iniciar el servidor",
		})
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, a.shutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.Error(ctx, err, map[string]interface{}{
			"message": "Error durante el shutdown del servidor",
		})
		return err
	}

	a.logger.Info(ctx, "Servidor apagado correctamente", nil)
	return nil
}

func registerPprofRoutes(router chi.Router) {
	router.Route("/debug/pprof", func(r chi.Router) {
		r.Get("/", http.HandlerFunc(pprof.Index))
		r.Get("/cmdline", http.HandlerFunc(pprof.Cmdline))
		r.Get("/profile", http.HandlerFunc(pprof.Profile))
		r.Get("/symbol", http.HandlerFunc(pprof.Symbol))
		r.Get("/trace", http.HandlerFunc(pprof.Trace))
		r.Get("/goroutine", http.HandlerFunc(pprof.Handler("goroutine").ServeHTTP))
		r.Get("/heap", http.HandlerFunc(pprof.Handler("heap").ServeHTTP))
		r.Get("/threadcreate", http.HandlerFunc(pprof.Handler("threadcreate").ServeHTTP))
		r.Get("/block", http.HandlerFunc(pprof.Handler("block").ServeHTTP))
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
