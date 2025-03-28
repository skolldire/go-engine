package app

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	"github.com/skolldire/go-engine/pkg/clients/sns"
	"github.com/skolldire/go-engine/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/config/viper"
	"github.com/skolldire/go-engine/pkg/database/dynamo"
	"github.com/skolldire/go-engine/pkg/database/gormsql"
	"github.com/skolldire/go-engine/pkg/database/redis"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"go.elastic.co/ecslogrus"
)

type clients struct {
	ctx       context.Context
	log       logger.Service
	awsConfig aws.Config
	errors    []error
}

func (c *App) GetConfigs() *App {
	tracer := logrus.New()
	tracer.SetOutput(os.Stdout)
	tracer.SetFormatter(&ecslogrus.Formatter{})
	tracer.Level = logrus.DebugLevel

	v := viper.NewService(tracer)
	conf, err := v.Apply()
	if err != nil {
		tracer.Error(err)
		c.Engine.errors = append(c.Engine.errors, err)
		return c
	}

	c.Engine.Conf = &conf
	log := setLogLevel(conf.Log, tracer)
	_ = log.SetLogLevel("trace")
	c.Engine.Log = log

	return c
}

func (c *App) Init() *App {
	if c.Engine.Conf == nil || len(c.Engine.errors) > 0 {
		return c
	}

	awsConfig, err := config.LoadDefaultConfig(c.Engine.ctx,
		config.WithRegion(c.Engine.Conf.Aws.Region),
	)
	if err != nil {
		c.Engine.errors = append(c.Engine.errors, err)
		return c
	}

	initializer := &clients{
		ctx:       c.Engine.ctx,
		log:       c.Engine.Log,
		awsConfig: awsConfig,
	}

	c.Engine.RestClients = initializer.createClientsHttp(c.Engine.Conf.Rest)
	c.Engine.SQSClient = initializer.createClientSQS(c.Engine.Conf.SQS)
	c.Engine.SNSClient = initializer.createClientSNS(c.Engine.Conf.SNS)
	c.Engine.DynamoDBClient = initializer.createClientDynamo(c.Engine.Conf.Dynamo)
	c.Engine.RedisClient = initializer.createClientRedis(c.Engine.Conf.Redis)
	c.Engine.SqlConnection = initializer.createClientSQL(c.Engine.Conf.DataBaseSql)
	c.Engine.RepositoriesConfig = c.Engine.Conf.Repositories
	c.Engine.UsesCasesConfig = c.Engine.Conf.Cases
	c.Engine.HandlerConfig = c.Engine.Conf.Endpoints
	c.Engine.BatchConfig = c.Engine.Conf.Processors

	if len(initializer.errors) > 0 {
		c.Engine.errors = append(c.Engine.errors, initializer.errors...)
	}

	return c
}

func (c *App) InitializeRouter() *App {
	if c.Engine.Conf == nil || len(c.Engine.errors) > 0 {
		return c
	}

	c.Engine.Router = router.NewService(c.Engine.Conf.Router, router.WithLogger(c.Engine.Log))
	_ = c.Engine.Log.SetLogLevel(c.Engine.Conf.Log.Level)
	return c
}

func (c *App) SetContext(ctx context.Context) *App {
	c.Engine.ctx = ctx
	return c
}

func (c *App) Build() *Engine {
	return c.Engine
}

func (i *clients) setError(err error) {
	i.errors = append(i.errors, err)
}

func (i *clients) createClientsHttp(configs []map[string]rest.Config) map[string]rest.Service {
	httpClients := make(map[string]rest.Service)
	for _, v := range configs {
		for k, cfg := range v {
			httpClients[k] = rest.NewClient(cfg, i.log)
		}
	}
	return httpClients
}

func (i *clients) createClientSQS(cfg *sqs.Config) sqs.Service {
	if cfg == nil {
		return nil
	}
	return sqs.NewClient(i.awsConfig, *cfg, i.log)
}

func (i *clients) createClientSNS(cfg *sns.Config) sns.Service {
	if cfg == nil {
		return nil
	}
	return sns.NewClient(i.awsConfig, *cfg, i.log)
}

func (i *clients) createClientDynamo(cfg *dynamo.Config) dynamo.Service {
	if cfg == nil {
		return nil
	}
	return dynamo.NewClient(i.awsConfig, *cfg, i.log)
}

func (i *clients) createClientRedis(cfg *redis.Config) *redis.RedisClient {
	if cfg == nil {
		return nil
	}

	client, err := redis.NewClient(*cfg, i.log)
	if err != nil {
		i.setError(err)
		return nil
	}

	return client
}

func (i *clients) createClientSQL(cfg *gormsql.Config) *gormsql.DBClient {
	if cfg == nil {
		return nil
	}

	client, err := gormsql.NewClient(*cfg, i.log)
	if err != nil {
		i.setError(err)
		return nil
	}

	return client
}

func setLogLevel(c logger.Config, l *logrus.Logger) logger.Service {
	return logger.NewService(logger.Config{
		Level: c.Level,
		Path:  c.Path,
	}, l)
}
