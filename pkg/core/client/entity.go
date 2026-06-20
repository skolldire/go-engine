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

// Operation is a unit of work passed to BaseClient.Execute.
// It must be idempotent when resilience (retry) is enabled.
type Operation func() (interface{}, error)

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
//	    result, err := c.Execute(ctx, "my-service.do-something", func() (interface{}, error) {
//	        return callExternalAPI()
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
	mu          sync.RWMutex // Protects logging and serviceName fields
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

	if config.WithResilience {
		bc.resilience = resilience.NewResilienceService(config.Resilience, log)
	}

	return bc
}

// Execute runs op under a timeout-bounded context, optionally logging start/end
// and wrapping op with the resilience layer when configured.
//
// Context handling:
//   - If ctx already has a deadline, Execute respects it unchanged.
//   - If ctx has no deadline, Execute applies bc.timeout.
//
// Resilience: when BaseConfig.WithResilience was true at construction, Execute
// delegates to the resilience.Service (retry + circuit breaker). The operation
// must be idempotent in that case.
//
// Return value: the raw interface{} returned by op. Use SafeTypeAssert[T] to
// convert it to a concrete type without a panic.
func (bc *BaseClient) Execute(ctx context.Context, operationName string, operation Operation) (interface{}, error) {
	ctx, cancel := bc.ensureContextWithTimeout(ctx)
	defer cancel()

	logFields := map[string]interface{}{
		"operation": operationName,
		"service":   bc.getServiceName(),
	}

	if bc.resilience != nil {
		return bc.executeWithResilience(ctx, operationName, operation, logFields)
	}

	return bc.executeDirectly(ctx, operationName, operation, logFields)
}

func (bc *BaseClient) executeWithResilience(ctx context.Context, operationName string, operation Operation, logFields map[string]interface{}) (interface{}, error) {
	bc.mu.RLock()
	logging := bc.logging
	bc.mu.RUnlock()

	if logging {
		bc.logger.Debug(ctx, "starting operation with resilience: "+operationName, logFields)
	}

	result, err := bc.resilience.Execute(ctx, operation)

	if err != nil && logging {
		bc.logger.Error(ctx, err, logFields)
	} else if logging {
		bc.logger.Debug(ctx, "operation completed with resilience: "+operationName, logFields)
	}

	return result, err
}

func (bc *BaseClient) executeDirectly(ctx context.Context, operationName string, operation Operation, logFields map[string]interface{}) (interface{}, error) {
	bc.mu.RLock()
	logging := bc.logging
	bc.mu.RUnlock()

	if logging {
		bc.logger.Debug(ctx, "starting operation: "+operationName, logFields)
	}

	result, err := operation()

	if err != nil && logging {
		bc.logger.Error(ctx, err, logFields)
	} else if logging {
		bc.logger.Debug(ctx, "operation completed: "+operationName, logFields)
	}

	return result, err
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
// Use this when an operation needs to pass the timeout-managed context into an
// SDK call: Operation receives no context, so the closure would otherwise close
// over an unbounded context. Bounding it before Execute keeps the operation's
// context consistent with the one Execute manages.
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
