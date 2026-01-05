package client

import (
	"context"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout = 10 * time.Second
)

type BaseConfig struct {
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
	Timeout        time.Duration     `mapstructure:"timeout" json:"timeout"`
}

type Operation func() (interface{}, error)

type BaseClient struct {
	logger      logger.Service
	logging     bool
	resilience  *resilience.Service
	timeout     time.Duration
	serviceName string
}

// NewBaseClient creates a BaseClient using the provided configuration and logger and sets the service name to "base".
func NewBaseClient(config BaseConfig, log logger.Service) *BaseClient {
	return NewBaseClientWithName(config, log, "base")
}

// NewBaseClientWithName creates a BaseClient configured with the provided logger and service name.
// The client's timeout is taken from config.Timeout; if zero, DefaultTimeout is applied.
// If config.WithResilience is true, a resilience service is initialized using config.Resilience and the provided logger.
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
	if bc.logging {
		bc.logger.Debug(ctx, "starting operation with resilience: "+operationName, logFields)
	}

	result, err := bc.resilience.Execute(ctx, operation)

	if err != nil && bc.logging {
		bc.logger.Error(ctx, err, logFields)
	} else if bc.logging {
		bc.logger.Debug(ctx, "operation completed with resilience: "+operationName, logFields)
	}

	return result, err
}

func (bc *BaseClient) executeDirectly(ctx context.Context, operationName string, operation Operation, logFields map[string]interface{}) (interface{}, error) {
	if bc.logging {
		bc.logger.Debug(ctx, "starting operation: "+operationName, logFields)
	}

	result, err := operation()

	if err != nil && bc.logging {
		bc.logger.Error(ctx, err, logFields)
	} else if bc.logging {
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

func (bc *BaseClient) SetLogging(enable bool) {
	bc.logging = enable
}

func (bc *BaseClient) IsLoggingEnabled() bool {
	return bc.logging
}

func (bc *BaseClient) GetLogger() logger.Service {
	return bc.logger
}

func (bc *BaseClient) getServiceName() string {
	if bc.serviceName != "" {
		return bc.serviceName
	}
	return "base"
}

func (bc *BaseClient) SetServiceName(name string) {
	bc.serviceName = name
}