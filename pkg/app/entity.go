package app

import (
	"context"
	"time"

	"github.com/skolldire/go-engine/pkg/app/build"
	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	"github.com/skolldire/go-engine/pkg/clients/sns"
	"github.com/skolldire/go-engine/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/database/dynamo"
	"github.com/skolldire/go-engine/pkg/database/gormsql"
	"github.com/skolldire/go-engine/pkg/database/redis"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type Engine struct {
	App                router.Service
	Log                logger.Service
	RestClients        map[string]rest.Service
	SQSClient          sqs.Service
	SNSClient          sns.Service
	DynamoDBClient     dynamo.Service
	RedisClient        *redis.RedisClient
	SqlConnection      *gormsql.DBClient
	HandlerConfig      map[string]interface{}
	BatchConfig        map[string]interface{}
	MiddlewareConfig   map[string]interface{}
	UsesCasesConfig    map[string]interface{}
	RepositoriesConfig map[string]interface{}
	errors             []error
	ctx                context.Context
	ShutdownTimeout    time.Duration
}

func (e *Engine) Run() error {
	if len(e.errors) > 0 {
		return e.errors[0]
	}

	if e.App != nil {
		return e.App.Run()
	}

	return nil
}

func (e *Engine) GetErrors() []error {
	return e.errors
}

var _ build.App = (*Engine)(nil)
