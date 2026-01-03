package app

import (
	"context"
	"fmt"

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
	grpcServer "github.com/skolldire/go-engine/pkg/server/grpc"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/telemetry"
)

type Engine struct {
	ctx                context.Context
	errors             []error
	Router             router.Service
	GrpcServer         grpcServer.Service
	Log                logger.Service
	Telemetry          telemetry.Telemetry
	Conf               *viper.Config
	RestClients        map[string]rest.Service
	GpcClients         map[string]grpcClient.Service
	SQSClient          sqs.Service
	SNSClient          sns.Service
	DynamoDBClient     dynamo.Service
	RedisClient        *redis.RedisClient
	SqlConnection      *gormsql.DBClient
	SQSClients         map[string]sqs.Service
	SNSClients         map[string]sns.Service
	DynamoDBClients    map[string]dynamo.Service
	RedisClients       map[string]*redis.RedisClient
	SQLConnections     map[string]*gormsql.DBClient
	SSMClients         map[string]ssm.Service
	SESClients         map[string]ses.Service
	S3Clients          map[string]s3.Service
	MemcachedClients   map[string]memcached.Service
	MongoDBClients     map[string]mongodb.Service
	RabbitMQClients    map[string]rabbitmq.Service
	RepositoriesConfig map[string]interface{}
	UsesCasesConfig    map[string]interface{}
	HandlerConfig      map[string]interface{}
	BatchConfig        map[string]interface{}
	FeatureFlags       *dynamic.FeatureFlags
	Validator          *validator.Validate
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
	if e.RestClients == nil {
		return nil
	}
	return e.RestClients[name]
}

func (e *Engine) GetGRPCClient(name string) grpcClient.Service {
	if e.GpcClients == nil {
		return nil
	}
	return e.GpcClients[name]
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

func (e *Engine) GetRepositoryConfig(name string) interface{} {
	if e.RepositoriesConfig == nil {
		return nil
	}
	return e.RepositoriesConfig[name]
}

func (e *Engine) GetUseCaseConfig(name string) interface{} {
	if e.UsesCasesConfig == nil {
		return nil
	}
	return e.UsesCasesConfig[name]
}

func (e *Engine) GetHandlerConfig(name string) interface{} {
	if e.HandlerConfig == nil {
		return nil
	}
	return e.HandlerConfig[name]
}

func (e *Engine) GetBatchConfig(name string) interface{} {
	if e.BatchConfig == nil {
		return nil
	}
	return e.BatchConfig[name]
}

func (e *Engine) GetSQSClientByName(name string) sqs.Service {
	if e.SQSClients == nil {
		return nil
	}
	return e.SQSClients[name]
}

func (e *Engine) GetSNSClientByName(name string) sns.Service {
	if e.SNSClients == nil {
		return nil
	}
	return e.SNSClients[name]
}

func (e *Engine) GetDynamoDBClientByName(name string) dynamo.Service {
	if e.DynamoDBClients == nil {
		return nil
	}
	return e.DynamoDBClients[name]
}

func (e *Engine) GetRedisClientByName(name string) *redis.RedisClient {
	if e.RedisClients == nil {
		return nil
	}
	return e.RedisClients[name]
}

func (e *Engine) GetSQLConnectionByName(name string) *gormsql.DBClient {
	if e.SQLConnections == nil {
		return nil
	}
	return e.SQLConnections[name]
}

func (e *Engine) GetFeatureFlags() *dynamic.FeatureFlags {
	return e.FeatureFlags
}

func (e *Engine) GetSSMClientByName(name string) ssm.Service {
	if e.SSMClients == nil {
		return nil
	}
	return e.SSMClients[name]
}

func (e *Engine) GetSESClientByName(name string) ses.Service {
	if e.SESClients == nil {
		return nil
	}
	return e.SESClients[name]
}

func (e *Engine) GetS3ClientByName(name string) s3.Service {
	if e.S3Clients == nil {
		return nil
	}
	return e.S3Clients[name]
}

func (e *Engine) GetMemcachedClientByName(name string) memcached.Service {
	if e.MemcachedClients == nil {
		return nil
	}
	return e.MemcachedClients[name]
}

func (e *Engine) GetMongoDBClientByName(name string) mongodb.Service {
	if e.MongoDBClients == nil {
		return nil
	}
	return e.MongoDBClients[name]
}

func (e *Engine) GetRabbitMQClientByName(name string) rabbitmq.Service {
	if e.RabbitMQClients == nil {
		return nil
	}
	return e.RabbitMQClients[name]
}

func (e *Engine) GetValidator() *validator.Validate {
	return e.Validator
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
