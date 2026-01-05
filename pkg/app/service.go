package app

import (
	"context"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/sirupsen/logrus"
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
	"github.com/skolldire/go-engine/pkg/integration/observability"
	grpcServer "github.com/skolldire/go-engine/pkg/server/grpc"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/telemetry"
	"github.com/skolldire/go-engine/pkg/utilities/validation"
	"go.elastic.co/ecslogrus"
)

type clients struct {
	ctx       context.Context
	log       logger.Service
	awsConfig awsconfig.Config
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

	awsCfg, err := config.LoadDefaultConfig(c.Engine.ctx,
		config.WithRegion(c.Engine.Conf.Aws.Region),
	)
	if err != nil {
		c.Engine.errors = append(c.Engine.errors, err)
		return c
	}

	initializer := &clients{
		ctx:       c.Engine.ctx,
		log:       c.Engine.Log,
		awsConfig: awsCfg,
	}

	c.Engine.GrpcServer = initializer.createServerGRPC(c.Engine.Conf.GrpcServer)

	// Initialize service registry using sync.Once to ensure thread-safety
	// and prevent overwriting if GetServices() was called first
	var existingCustomClients map[string]interface{}
	if c.Engine.Services != nil && c.Engine.Services.CustomClients != nil {
		existingCustomClients = c.Engine.Services.CustomClients
	}

	c.Engine.servicesOnce.Do(func() {
		c.Engine.Services = NewServiceRegistry()
	})

	// Restore existing custom clients if they were present
	if existingCustomClients != nil {
		c.Engine.Services.CustomClients = existingCustomClients
	}

	c.Engine.Services.RESTClients = initializer.createClientsHttp(c.Engine.Conf.Rest)
	c.Engine.Services.GRPCClients = initializer.createClientGRPC(c.Engine.Conf.GrpcClient)

	// Legacy single clients (for backward compatibility)
	c.Engine.SQSClient = initializer.createClientSQS(c.Engine.Conf.SQS)
	c.Engine.SNSClient = initializer.createClientSNS(c.Engine.Conf.SNS)
	c.Engine.DynamoDBClient = initializer.createClientDynamo(c.Engine.Conf.Dynamo)
	c.Engine.RedisClient = initializer.createClientRedis(c.Engine.Conf.Redis)
	c.Engine.SqlConnection = initializer.createClientSQL(c.Engine.Conf.DataBaseSql)

	// Multiple clients in registry
	c.Engine.Services.SQSClients = initializer.createClientsSQS(c.Engine.Conf.SQSClients)
	c.Engine.Services.SNSClients = initializer.createClientsSNS(c.Engine.Conf.SNSClients)
	c.Engine.Services.DynamoDBClients = initializer.createClientsDynamo(c.Engine.Conf.DynamoClients)
	c.Engine.Services.RedisClients = initializer.createClientsRedis(c.Engine.Conf.RedisClients)
	c.Engine.Services.SQLConnections = initializer.createClientsSQL(c.Engine.Conf.SQLConnections)
	c.Engine.Services.SSMClients = initializer.createClientsSSM(c.Engine.Conf.SSMClients)
	c.Engine.Services.SESClients = initializer.createClientsSES(c.Engine.Conf.SESClients)
	c.Engine.Services.S3Clients = initializer.createClientsS3(c.Engine.Conf.S3Clients)
	c.Engine.Services.MemcachedClients = initializer.createClientsMemcached(c.Engine.Conf.MemcachedClients)
	c.Engine.Services.MongoDBClients = initializer.createClientsMongoDB(c.Engine.Conf.MongoDBClients)
	c.Engine.Services.RabbitMQClients = initializer.createClientsRabbitMQ(c.Engine.Conf.RabbitMQClients)

	c.Engine.Telemetry = initializer.createTelemetry(c.Engine.Conf.Telemetry)

	// Initialize CloudClient (optional - can be nil if not configured)
	c.Engine.CloudClient = initializer.createCloudClient(c.Engine.Log, c.Engine.Telemetry)

	// Initialize config registry using sync.Once to ensure thread-safety
	// and prevent overwriting if GetConfigs() was called first
	c.Engine.configsOnce.Do(func() {
		c.Engine.Configs = NewConfigRegistry()
	})
	c.Engine.Configs.Repositories = c.Engine.Conf.Repositories
	c.Engine.Configs.UseCases = c.Engine.Conf.Cases
	c.Engine.Configs.Handlers = c.Engine.Conf.Endpoints
	c.Engine.Configs.Batches = c.Engine.Conf.Processors

	c.Engine.Validator = validation.NewValidator()
	validation.SetGlobalValidator(c.Engine.Validator)

	if c.Engine.Conf.FeatureFlags != nil {
		c.Engine.FeatureFlags = dynamic.NewFeatureFlags(c.Engine.Conf.FeatureFlags, c.Engine.Log)
	}

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

func (i *clients) createServerGRPC(cfg *grpcServer.Config) grpcServer.Service {
	if cfg == nil {
		return nil
	}
	return grpcServer.NewServer(*cfg, i.log)
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

func (i *clients) createClientGRPC(configs []map[string]grpcClient.Config) map[string]grpcClient.Service {
	grpcClients := make(map[string]grpcClient.Service)
	for _, v := range configs {
		for k, cfg := range v {
			client, err := grpcClient.NewCliente(cfg, i.log)
			if err != nil {
				i.setError(err)
				continue
			}
			grpcClients[k] = client
		}
	}
	return grpcClients
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

func (i *clients) createTelemetry(cfg *telemetry.Config) telemetry.Telemetry {
	tel, err := telemetry.NewTelemetry(i.ctx, *cfg)
	if err != nil {
		i.setError(err)
		return nil
	}
	return tel
}

func (i *clients) createCloudClient(log logger.Service, tel telemetry.Telemetry) awsclient.Client {
	// Create CloudClient - always available if AWS config exists
	// Observability can be added later via middleware if needed
	var metricsRecorder observability.MetricsRecorder
	if tel != nil {
		metricsRecorder = observability.NewTelemetryMetricsRecorder(tel)
	}
	return awsclient.NewWithOptions(i.awsConfig, awsclient.WithObservability(log, metricsRecorder, tel))
}

func (i *clients) createClientsSQS(configs []map[string]sqs.Config) map[string]sqs.Service {
	sqsClients := make(map[string]sqs.Service)
	for _, v := range configs {
		for k, cfg := range v {
			sqsClients[k] = sqs.NewClient(i.awsConfig, cfg, i.log)
		}
	}
	return sqsClients
}

func (i *clients) createClientsSNS(configs []map[string]sns.Config) map[string]sns.Service {
	snsClients := make(map[string]sns.Service)
	for _, v := range configs {
		for k, cfg := range v {
			snsClients[k] = sns.NewClient(i.awsConfig, cfg, i.log)
		}
	}
	return snsClients
}

func (i *clients) createClientsDynamo(configs []map[string]dynamo.Config) map[string]dynamo.Service {
	dynamoClients := make(map[string]dynamo.Service)
	for _, v := range configs {
		for k, cfg := range v {
			dynamoClients[k] = dynamo.NewClient(i.awsConfig, cfg, i.log)
		}
	}
	return dynamoClients
}

func (i *clients) createClientsRedis(configs []map[string]redis.Config) map[string]*redis.RedisClient {
	redisClients := make(map[string]*redis.RedisClient)
	for _, v := range configs {
		for k, cfg := range v {
			client, err := redis.NewClient(cfg, i.log)
			if err != nil {
				i.setError(err)
				continue
			}
			redisClients[k] = client
		}
	}
	return redisClients
}

func (i *clients) createClientsSQL(configs []map[string]gormsql.Config) map[string]*gormsql.DBClient {
	sqlConnections := make(map[string]*gormsql.DBClient)
	for _, v := range configs {
		for k, cfg := range v {
			client, err := gormsql.NewClient(cfg, i.log)
			if err != nil {
				i.setError(err)
				continue
			}
			sqlConnections[k] = client
		}
	}
	return sqlConnections
}

func (i *clients) createClientsSSM(configs []map[string]ssm.Config) map[string]ssm.Service {
	ssmClients := make(map[string]ssm.Service)
	for _, v := range configs {
		for k, cfg := range v {
			ssmClients[k] = ssm.NewClient(i.awsConfig, cfg, i.log)
		}
	}
	return ssmClients
}

func (i *clients) createClientsSES(configs []map[string]ses.Config) map[string]ses.Service {
	sesClients := make(map[string]ses.Service)
	for _, v := range configs {
		for k, cfg := range v {
			sesClients[k] = ses.NewClient(i.awsConfig, cfg, i.log)
		}
	}
	return sesClients
}

func (i *clients) createClientsS3(configs []map[string]s3.Config) map[string]s3.Service {
	s3Clients := make(map[string]s3.Service)
	for _, v := range configs {
		for k, cfg := range v {
			s3Clients[k] = s3.NewClient(i.awsConfig, cfg, i.log)
		}
	}
	return s3Clients
}

func (i *clients) createClientsMemcached(configs []map[string]memcached.Config) map[string]memcached.Service {
	memcachedClients := make(map[string]memcached.Service)
	for _, v := range configs {
		for k, cfg := range v {
			client, err := memcached.NewClient(cfg, i.log)
			if err != nil {
				i.setError(err)
				continue
			}
			memcachedClients[k] = client
		}
	}
	return memcachedClients
}

func (i *clients) createClientsMongoDB(configs []map[string]mongodb.Config) map[string]mongodb.Service {
	mongoDBClients := make(map[string]mongodb.Service)
	for _, v := range configs {
		for k, cfg := range v {
			client, err := mongodb.NewClient(cfg, i.log)
			if err != nil {
				i.setError(err)
				continue
			}
			mongoDBClients[k] = client
		}
	}
	return mongoDBClients
}

func (i *clients) createClientsRabbitMQ(configs []map[string]rabbitmq.Config) map[string]rabbitmq.Service {
	rabbitMQClients := make(map[string]rabbitmq.Service)
	for _, v := range configs {
		for k, cfg := range v {
			client, err := rabbitmq.NewClient(cfg, i.log)
			if err != nil {
				i.setError(err)
				continue
			}
			rabbitMQClients[k] = client
		}
	}
	return rabbitMQClients
}

// setLogLevel creates a logger service configured with the provided logging level and path using the given Logrus logger.
// The returned service wraps the provided *logrus.Logger and applies c.Level and c.Path to the logger configuration.
func setLogLevel(c logger.Config, l *logrus.Logger) logger.Service {
	return logger.NewService(logger.Config{
		Level: c.Level,
		Path:  c.Path,
	}, l)
}