package router

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

const (
	appDefaultPort         = "8080"
	defaultReadTimeout     = 10 * time.Second
	defaultWriteTimeout    = 30 * time.Second
	defaultIdleTimeout     = 120 * time.Second
	defaultShutdownTimeout = 30 * time.Second
	// defaultHandlerTimeout matches defaultWriteTimeout on purpose: a handler
	// budget larger than the write timeout means the server gives up on the
	// response before the handler is cut off, which the previous hardcoded 60s
	// (against a 30s write timeout) did on every slow request.
	defaultHandlerTimeout = defaultWriteTimeout
)

// Service is the public interface for the HTTP router returned by NewService.
// Consumers use it to register routes, middleware, and shutdown hooks.
type Service interface {
	Run(ctx context.Context) error
	Use(middlewares ...func(http.Handler) http.Handler)
	Mount(pattern string, handler http.Handler)
	AddRoute(method, pattern string, handler http.HandlerFunc)
	Router() *chi.Mux
	RegisterShutdownHook(fn func(context.Context) error)
}

// App is the concrete implementation of Service backed by chi.
type App struct {
	router          *chi.Mux
	server          *http.Server
	config          Config
	shutdownTimeout time.Duration
	logger          logger.Service
	hooksMu         sync.Mutex
	shutdownHooks   []func(context.Context) error
}

// Config holds the HTTP server settings populated from the `router:` YAML section.
type Config struct {
	Port            string        `mapstructure:"port" json:"port"`
	Name            string        `mapstructure:"name" json:"name"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout" json:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout" json:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout" json:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" json:"shutdown_timeout"`
	// HandlerTimeout bounds how long a single handler may run before the
	// middleware cancels its request context. Defaults to WriteTimeout.
	HandlerTimeout time.Duration `mapstructure:"handler_timeout" json:"handler_timeout"`
	EnableCORS     bool          `mapstructure:"enable_cors" json:"enable_cors"`
	CorsConfig     Cors          `mapstructure:"cors_config" json:"cors_config"`
	TrustedProxies []string      `mapstructure:"trusted_proxies" json:"trusted_proxies"`
	// EnablePprof exposes the net/http/pprof endpoints under /debug/pprof.
	// It is opt-in and additionally suppressed under the production profile so
	// profiling data is never exposed in prod even if enabled by mistake.
	EnablePprof bool `mapstructure:"enable_pprof" json:"enable_pprof"`
}

// Cors holds the CORS policy applied when Config.EnableCORS is true.
type Cors struct {
	AllowOrigins     []string `mapstructure:"allow_origins" json:"allow_origins"`
	AllowMethods     []string `mapstructure:"allow_methods" json:"allow_methods"`
	AllowHeaders     []string `mapstructure:"allow_headers" json:"allow_headers"`
	ExposedHeaders   []string `mapstructure:"exposed_headers" json:"exposed_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials" json:"allow_credentials"`
	AllowMaxAge      int      `mapstructure:"allow_max_age" json:"allow_max_age"`
}

// RouterOption is a functional option applied to App during construction.
type RouterOption func(*App)
