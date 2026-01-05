package client

import (
	"context"
	"sync"
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
	mu          sync.RWMutex // Protects logging and serviceName fields
}

func NewBaseClient(config BaseConfig, log logger.Service) *BaseClient {
	return NewBaseClientWithName(config, log, "base")
}

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

func (bc *BaseClient) SetLogging(enable bool) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.logging = enable
}

func (bc *BaseClient) IsLoggingEnabled() bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.logging
}

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

func (bc *BaseClient) SetServiceName(name string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.serviceName = name
}
