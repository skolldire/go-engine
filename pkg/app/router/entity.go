package router

import (
	"context"
	"net/http"
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
)

// Service is the public interface for the HTTP router returned by NewService.
// Consumers use it to register routes, middleware, and shutdown hooks.
type Service interface {
	Run() error
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
	EnableCORS      bool          `mapstructure:"enable_cors" json:"enable_cors"`
	CorsConfig      Cors          `mapstructure:"cors_config" json:"cors_config"`
	TrustedProxies  []string      `mapstructure:"trusted_proxies" json:"trusted_proxies"`
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
