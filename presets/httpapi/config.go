// Package httpapi assembles explicit HTTP handlers and owns only its HTTP servers.
// It constructs no engine or external resources; see README.md for assembly.
package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/skolldire/go-engine/router"
	"github.com/skolldire/go-engine/router/middleware"
	"github.com/skolldire/go-engine/router/operations"
)

var (
	// ErrInvalidConfig identifies invalid preset configuration/dependencies.
	ErrInvalidConfig = errors.New("invalid httpapi configuration")
	// ErrInvalidCall identifies invalid Run/RunListeners arguments.
	ErrInvalidCall = errors.New("invalid httpapi call")
	// ErrAlreadyStarted identifies a second valid execution attempt.
	ErrAlreadyStarted = errors.New("httpapi already started")
)

// ConfigError names the invalid field without its value and wraps ErrInvalidConfig.
type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "httpapi: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

// RequestIDConfig controls API request IDs; generation is a borrowed dependency.
type RequestIDConfig struct {
	Enabled        bool   `mapstructure:"enabled" json:"enabled"`
	Header         string `mapstructure:"header" json:"header"`
	AcceptIncoming bool   `mapstructure:"accept_incoming" json:"accept_incoming"`
}

// Route is one borrowed business handler registered before the mux is frozen.
type Route struct {
	Method, Host, Pattern string
	Handler               http.Handler
}

// Config is literal policy. Disabled operation settings are ignored.
type Config struct {
	Router           router.Config       `mapstructure:"router" json:"router"`
	Server           router.ServerConfig `mapstructure:"server" json:"server"`
	EnableOperations bool                `mapstructure:"enable_operations" json:"enable_operations"`
	OperationsRouter router.Config       `mapstructure:"operations_router" json:"operations_router"`
	OperationsServer router.ServerConfig `mapstructure:"operations_server" json:"operations_server"`
	Operations       operations.Config   `mapstructure:"operations" json:"operations"`
	RequestID        RequestIDConfig     `mapstructure:"request_id" json:"request_id"`
	EnableRecovery   bool                `mapstructure:"enable_recovery" json:"enable_recovery"`
	RequestTimeout   time.Duration       `mapstructure:"request_timeout" json:"request_timeout"`
}

// Dependencies are borrowed; callbacks and handlers must be safe for concurrent serving.
type Dependencies struct {
	Routes               []Route
	Middleware           []router.Middleware
	OperationsMiddleware []router.Middleware
	Health               operations.Health
	Metrics              http.Handler
	GenerateRequestID    func() string
	OnPanic              func(http.ResponseWriter, *http.Request, any)
}

// DefaultConfig returns explicit defaults; host allowlists and Listen remain required.
func DefaultConfig() Config {
	opsServer := router.DefaultServerConfig()
	opsServer.WriteTimeout = 45 * time.Second
	return Config{Server: router.DefaultServerConfig(), EnableOperations: true, OperationsServer: opsServer, Operations: operations.DefaultConfig(), RequestID: RequestIDConfig{Enabled: true, Header: "X-Request-ID"}, EnableRecovery: true}
}
func delegated(field string, err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(&ConfigError{field, "invalid delegated configuration"}, err)
}

// Validate delegates tool validation without callbacks/I/O and checks cross-budgets.
func (c Config) Validate() error {
	if _, e := router.NewPolicy(c.Router); e != nil {
		return delegated("Router", e)
	}
	if e := c.Server.Validate(); e != nil {
		return delegated("Server", e)
	}
	if c.EnableOperations {
		if _, e := router.NewPolicy(c.OperationsRouter); e != nil {
			return delegated("OperationsRouter", e)
		}
		if e := c.OperationsServer.Validate(); e != nil {
			return delegated("OperationsServer", e)
		}
		if e := c.Operations.Validate(); e != nil {
			return delegated("Operations", e)
		}
	}
	if c.RequestID.Enabled {
		if _, e := middleware.RequestID(middleware.RequestIDConfig{Header: c.RequestID.Header}); e != nil {
			return delegated("RequestID", e)
		}
	}
	if c.RequestTimeout < 0 {
		return &ConfigError{"RequestTimeout", "must be nonnegative"}
	}
	if c.RequestTimeout > 0 && c.Server.WriteTimeout > 0 && c.Server.WriteTimeout <= c.RequestTimeout {
		return &ConfigError{"Server.WriteTimeout", "must exceed RequestTimeout or be unlimited"}
	}
	if c.EnableOperations && c.Operations.Pprof && c.OperationsServer.WriteTimeout > 0 && c.OperationsServer.WriteTimeout <= c.Operations.MaxProfileDuration {
		return &ConfigError{"OperationsServer.WriteTimeout", "must exceed MaxProfileDuration or be unlimited"}
	}
	return nil
}
func copyPolicy(c router.Config) router.Config {
	c.AllowedHosts = append([]string(nil), c.AllowedHosts...)
	c.TrustedProxies = append([]string(nil), c.TrustedProxies...)
	if c.CORS != nil {
		x := *c.CORS
		x.Origins = append([]string(nil), x.Origins...)
		x.Methods = append([]string(nil), x.Methods...)
		x.Headers = append([]string(nil), x.Headers...)
		x.ExposedHeaders = append([]string(nil), x.ExposedHeaders...)
		c.CORS = &x
	}
	return c
}
