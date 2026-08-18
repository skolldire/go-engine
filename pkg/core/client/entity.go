package client

import (
	"context"
	"sync"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	// DefaultTimeout is the timeout applied to every operation when the caller
	// does not provide a context with a deadline and the BaseConfig.Timeout is zero.
	DefaultTimeout = 10 * time.Second
)

// BaseConfig holds the cross-cutting settings shared by all clients that embed BaseClient.
// It is typically populated from the application's YAML configuration via mapstructure.
type BaseConfig struct {
	// EnableLogging controls whether debug and error log entries are emitted
	// for every operation executed through BaseClient.Execute.
	EnableLogging bool `mapstructure:"enable_logging" json:"enable_logging"`

	// WithResilience enables the resilience layer (retry + circuit breaker).
	// When true, the Resilience field must be populated.
	WithResilience bool `mapstructure:"with_resilience" json:"with_resilience"`

	// Resilience holds the retry and circuit-breaker configuration used when
	// WithResilience is true.
	Resilience resilience.Config `mapstructure:"resilience" json:"resilience"`

	// Timeout is the maximum duration allowed for a single operation.
	// If zero, DefaultTimeout (10 s) is used. This timeout is applied only
	// when the caller's context has no deadline; if the context already has
	// a deadline, that deadline is respected as-is.
	Timeout time.Duration `mapstructure:"timeout" json:"timeout"`
}

// Operation is a unit of work passed to BaseClient.Execute. It receives the
// timeout-bounded context that Execute manages and MUST use it for any I/O so
// the configured timeout actually cancels the underlying call. It must be
// idempotent when resilience (retry) is enabled.
type Operation func(ctx context.Context) (any, error)

// BaseClient is an embeddable struct that provides logging, timeout management,
// and optional resilience (retry + circuit breaker) to any client implementation.
//
// Typical usage:
//
//	type MyClient struct {
//	    client.BaseClient
//	    // ... your fields
//	}
//
//	func (c *MyClient) DoSomething(ctx context.Context) (string, error) {
//	    result, err := c.Execute(ctx, "my-service.do-something", func(ctx context.Context) (any, error) {
//	        return callExternalAPI(ctx)
//	    })
//	    if err != nil {
//	        return "", err
//	    }
//	    return client.SafeTypeAssert[string](result)
//	}
//
// BaseClient is safe for concurrent use; its mutable fields are protected by an
// internal RWMutex.
type BaseClient struct {
	logger      logger.Service
	logging     bool
	resilience  *resilience.Service
	timeout     time.Duration
	serviceName string

	// chain is the composed middleware. Cross-cutting concerns live here rather
	// than as conditionals inside Execute, so adding metrics or tracing is a
	// composition change instead of an edit to every client package.
	chain Handler

	mu sync.RWMutex // Protects logging, serviceName and chain
}

// NewBaseClient creates a BaseClient with service name "base".
// See NewBaseClientWithName for full documentation.
func NewBaseClient(config BaseConfig, log logger.Service) *BaseClient {
	return NewBaseClientWithName(config, log, "base")
}

// NewBaseClientWithName creates a BaseClient with the given service name.
// The service name appears in log fields as "service" to distinguish log
// entries from different client types.
//
// If config.Timeout is zero, DefaultTimeout is used.
// If config.WithResilience is true, a resilience.Service is initialised from
// config.Resilience; otherwise no retry or circuit-breaker is applied.
func NewBaseClientWithName(config BaseConfig, log logger.Service, serviceName string) *BaseClient {
	bc := &BaseClient{
		logger:      log,
		logging:     config.EnableLogging,
		timeout:     config.Timeout,
		serviceName: serviceName,
	}

	if config.Timeout == 0 {
		bc.timeout = DefaultTimeout
	}

	// The default chain reproduces the previous behaviour exactly: logging when
	// EnableLogging is set, resilience when WithResilience is set. Callers that
	// want metrics or tracing replace it with Use.
	var mw []Middleware
	if log != nil {
		mw = append(mw, WithLogging(log, bc.IsLoggingEnabled))
	}
	if config.WithResilience {
		bc.resilience = resilience.NewResilienceService(config.Resilience, log)
		mw = append(mw, WithResilienceService(bc.resilience))
	}
	bc.chain = Chain(mw...)(baseHandler)

	return bc
}

// Execute runs op under a timeout-bounded context, optionally logging start/end
// and wrapping op with the resilience layer when configured.
//
// Context handling:
//   - If ctx already has a deadline, Execute respects it unchanged.
//   - If ctx has no deadline, Execute applies bc.timeout.
//
// The resulting bounded context is passed to the operation, so operations must
// use it for their I/O for the timeout to actually cancel the underlying call.
//
// Resilience: when BaseConfig.WithResilience was true at construction, Execute
// delegates to the resilience.Service (retry + circuit breaker). The operation
// must be idempotent in that case.
//
// Return value: the raw any returned by op. Use SafeTypeAssert[T] to
// convert it to a concrete type without a panic.
func (bc *BaseClient) Execute(ctx context.Context, operationName string, operation Operation) (any, error) {
	ctx, cancel := bc.ensureContextWithTimeout(ctx)
	defer cancel()

	return bc.chain(ctx, Invocation{
		Client:    bc.getServiceName(),
		Operation: operationName,
	}, operation)
}

// Use replaces the middleware chain. The default chain is built from
// BaseConfig; this is for clients that need to add metrics, tracing, or their
// own concern without every client package growing its own conditional.
//
// Middleware is applied outermost-first.
func (bc *BaseClient) Use(mw ...Middleware) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.chain = Chain(mw...)(baseHandler)
}

func (bc *BaseClient) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, bc.timeout)
}

// ContextWithTimeout returns a context derived from ctx and bounded by the
// client's timeout (DefaultTimeout if none was configured). If ctx already has
// a deadline, that deadline is respected as-is. The caller must invoke the
// returned cancel function.
//
// Execute already passes the bounded context to the operation, so most code
// does not need this. It remains available for callers that must bound a
// context outside of an Execute call.
func (bc *BaseClient) ContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return bc.ensureContextWithTimeout(ctx)
}

// SetLogging enables or disables operation logging at runtime. Safe for concurrent use.
func (bc *BaseClient) SetLogging(enable bool) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.logging = enable
}

// IsLoggingEnabled reports whether operation logging is currently active. Safe for concurrent use.
func (bc *BaseClient) IsLoggingEnabled() bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.logging
}

// GetLogger returns the logger.Service injected at construction time.
func (bc *BaseClient) GetLogger() logger.Service {
	return bc.logger
}

func (bc *BaseClient) getServiceName() string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if bc.serviceName != "" {
		return bc.serviceName
	}
	return "base"
}

// SetServiceName replaces the service name used in log fields. Safe for concurrent use.
func (bc *BaseClient) SetServiceName(name string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.serviceName = name
}
