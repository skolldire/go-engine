package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/skolldire/go-engine/pkg/app/router"
	grpcClient "github.com/skolldire/go-engine/pkg/clients/grpc"
	"github.com/skolldire/go-engine/pkg/clients/rabbitmq"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	"github.com/skolldire/go-engine/pkg/clients/s3"
	"github.com/skolldire/go-engine/pkg/clients/ses"
	"github.com/skolldire/go-engine/pkg/clients/sns"
	"github.com/skolldire/go-engine/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/clients/ssm"
	"github.com/skolldire/go-engine/pkg/config/dynamic"
	"github.com/skolldire/go-engine/pkg/config/viper"
	"github.com/skolldire/go-engine/pkg/database/dynamo"
	"github.com/skolldire/go-engine/pkg/database/gormsql"
	"github.com/skolldire/go-engine/pkg/database/memcached"
	"github.com/skolldire/go-engine/pkg/database/mongodb"
	"github.com/skolldire/go-engine/pkg/database/redis"
	awsclient "github.com/skolldire/go-engine/pkg/integration/aws"
	grpcServer "github.com/skolldire/go-engine/pkg/server/grpc"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/telemetry"
)

type Engine struct {
	ctx        context.Context
	errors     []error
	Router     router.Service
	GrpcServer grpcServer.Service
	Log        logger.Service
	Telemetry  telemetry.Telemetry
	Conf       *viper.Config

	// Legacy single clients (deprecated, use Services instead)
	SQSClient      sqs.Service
	SNSClient      sns.Service
	DynamoDBClient dynamo.Service
	RedisClient    *redis.RedisClient
	SqlConnection  *gormsql.DBClient

	// Service registry (composition pattern)
	Services *ServiceRegistry

	// Config registry (composition pattern)
	Configs *ConfigRegistry

	// Synchronization for lazy initialization
	servicesOnce sync.Once
	configsOnce  sync.Once

	// Feature flags and validation
	FeatureFlags *dynamic.FeatureFlags
	Validator    *validator.Validate

	// Cloud integration client
	CloudClient awsclient.Client // Optional: HTTP-like AWS integration facade
}

func (e *Engine) GetErrors() []error {
	return e.errors
}

func (e *Engine) Run() error {
	if e.Router == nil {
		return fmt.Errorf("router not initialized")
	}
	return e.Router.Run()
}

func (e *Engine) GetContext() context.Context {
	return e.ctx
}

func (e *Engine) GetRouter() router.Service {
	return e.Router
}

func (e *Engine) GetLogger() logger.Service {
	return e.Log
}

func (e *Engine) GetConfig() *viper.Config {
	return e.Conf
}

func (e *Engine) GetRestClient(name string) rest.Service {
	if e.Services == nil || e.Services.RESTClients == nil {
		return nil
	}
	return e.Services.RESTClients[name]
}

func (e *Engine) GetGRPCClient(name string) grpcClient.Service {
	if e.Services == nil || e.Services.GRPCClients == nil {
		return nil
	}
	return e.Services.GRPCClients[name]
}

func (e *Engine) GetSQSClient() sqs.Service {
	return e.SQSClient
}

func (e *Engine) GetSNSClient() sns.Service {
	return e.SNSClient
}

func (e *Engine) GetDynamoDBClient() dynamo.Service {
	return e.DynamoDBClient
}

func (e *Engine) GetRedisClient() *redis.RedisClient {
	return e.RedisClient
}

func (e *Engine) GetSQLConnection() *gormsql.DBClient {
	return e.SqlConnection
}

func (e *Engine) GetGRPCServer() grpcServer.Service {
	return e.GrpcServer
}

func (e *Engine) GetTelemetry() telemetry.Telemetry {
	return e.Telemetry
}

// GetServices returns the service registry
// Thread-safe lazy initialization using sync.Once
func (e *Engine) GetServices() *ServiceRegistry {
	e.servicesOnce.Do(func() {
		e.Services = NewServiceRegistry()
	})
	return e.Services
}

// GetConfigs returns the config registry
// Thread-safe lazy initialization using sync.Once
func (e *Engine) GetConfigs() *ConfigRegistry {
	e.configsOnce.Do(func() {
		e.Configs = NewConfigRegistry()
	})
	return e.Configs
}

// Legacy getters for backward compatibility
func (e *Engine) GetRepositoryConfig(name string) interface{} {
	if e.Configs == nil || e.Configs.Repositories == nil {
		return nil
	}
	return e.Configs.Repositories[name]
}

func (e *Engine) GetUseCaseConfig(name string) interface{} {
	if e.Configs == nil || e.Configs.UseCases == nil {
		return nil
	}
	return e.Configs.UseCases[name]
}

func (e *Engine) GetHandlerConfig(name string) interface{} {
	if e.Configs == nil || e.Configs.Handlers == nil {
		return nil
	}
	return e.Configs.Handlers[name]
}

func (e *Engine) GetBatchConfig(name string) interface{} {
	if e.Configs == nil || e.Configs.Batches == nil {
		return nil
	}
	return e.Configs.Batches[name]
}

func (e *Engine) GetSQSClientByName(name string) sqs.Service {
	if e.Services == nil || e.Services.SQSClients == nil {
		return nil
	}
	return e.Services.SQSClients[name]
}

func (e *Engine) GetSNSClientByName(name string) sns.Service {
	if e.Services == nil || e.Services.SNSClients == nil {
		return nil
	}
	return e.Services.SNSClients[name]
}

func (e *Engine) GetDynamoDBClientByName(name string) dynamo.Service {
	if e.Services == nil || e.Services.DynamoDBClients == nil {
		return nil
	}
	return e.Services.DynamoDBClients[name]
}

func (e *Engine) GetRedisClientByName(name string) *redis.RedisClient {
	if e.Services == nil || e.Services.RedisClients == nil {
		return nil
	}
	return e.Services.RedisClients[name]
}

func (e *Engine) GetSQLConnectionByName(name string) *gormsql.DBClient {
	if e.Services == nil || e.Services.SQLConnections == nil {
		return nil
	}
	return e.Services.SQLConnections[name]
}

func (e *Engine) GetFeatureFlags() *dynamic.FeatureFlags {
	return e.FeatureFlags
}

func (e *Engine) GetSSMClientByName(name string) ssm.Service {
	if e.Services == nil || e.Services.SSMClients == nil {
		return nil
	}
	return e.Services.SSMClients[name]
}

func (e *Engine) GetSESClientByName(name string) ses.Service {
	if e.Services == nil || e.Services.SESClients == nil {
		return nil
	}
	return e.Services.SESClients[name]
}

func (e *Engine) GetS3ClientByName(name string) s3.Service {
	if e.Services == nil || e.Services.S3Clients == nil {
		return nil
	}
	return e.Services.S3Clients[name]
}

func (e *Engine) GetMemcachedClientByName(name string) memcached.Service {
	if e.Services == nil || e.Services.MemcachedClients == nil {
		return nil
	}
	return e.Services.MemcachedClients[name]
}

func (e *Engine) GetMongoDBClientByName(name string) mongodb.Service {
	if e.Services == nil || e.Services.MongoDBClients == nil {
		return nil
	}
	return e.Services.MongoDBClients[name]
}

func (e *Engine) GetRabbitMQClientByName(name string) rabbitmq.Service {
	if e.Services == nil || e.Services.RabbitMQClients == nil {
		return nil
	}
	return e.Services.RabbitMQClients[name]
}

func (e *Engine) GetValidator() *validator.Validate {
	return e.Validator
}

func (e *Engine) GetCloudClient() awsclient.Client {
	return e.CloudClient
}

// GetCustomClient retrieves a custom client by name
func (e *Engine) GetCustomClient(name string) interface{} {
	if e.Services == nil || e.Services.CustomClients == nil {
		return nil
	}
	return e.Services.CustomClients[name]
}

type App struct {
	Engine *Engine
}

func NewApp() *App {
	return &App{
		Engine: &Engine{
			errors: []error{},
			ctx:    context.Background(),
		},
	}
}
