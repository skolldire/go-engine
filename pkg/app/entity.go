package app

import (
	"context"

	"github.com/skolldire/go-engine/pkg/app/router"
	grpcClient "github.com/skolldire/go-engine/pkg/clients/grpc"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	"github.com/skolldire/go-engine/pkg/clients/sns"
	"github.com/skolldire/go-engine/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/config/viper"
	"github.com/skolldire/go-engine/pkg/database/dynamo"
	"github.com/skolldire/go-engine/pkg/database/gormsql"
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
	RepositoriesConfig map[string]interface{}
	UsesCasesConfig    map[string]interface{}
	HandlerConfig      map[string]interface{}
	BatchConfig        map[string]interface{}
}

func (e *Engine) GetErrors() []error {
	return e.errors
}

func (e *Engine) Run() error {
	return e.Router.Run()
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
