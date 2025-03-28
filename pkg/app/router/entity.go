package router

import (
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

type Service interface {
	Run() error
	Use(middlewares ...func(http.Handler) http.Handler)
	Mount(pattern string, handler http.Handler)
	AddRoute(method, pattern string, handler http.HandlerFunc)
	Router() *chi.Mux
}

type App struct {
	router          *chi.Mux
	server          *http.Server
	config          Config
	shutdownTimeout time.Duration
	logger          logger.Service
}

type Config struct {
	Port             string        `mapstructure:"port"`
	Name             string        `mapstructure:"name"`
	ReadTimeout      time.Duration `mapstructure:"read_timeout"`
	WriteTimeout     time.Duration `mapstructure:"write_timeout"`
	IdleTimeout      time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout  time.Duration `mapstructure:"shutdown_timeout"`
	EnableCORS       bool          `mapstructure:"enable_cors"`
	AllowOrigins     []string      `mapstructure:"allow_origins"`
	AllowMethods     []string      `mapstructure:"allow_methods"`
	AllowHeaders     []string      `mapstructure:"allow_headers"`
	ExposedHeaders   []string      `mapstructure:"exposed_headers"`
	AllowCredentials bool          `mapstructure:"allow_credentials"`
	AllowMaxAge      int           `mapstructure:"allow_max_age"`
	TrustedProxies   []string      `mapstructure:"trusted_proxies"`
}

type RouterOption func(*App)
