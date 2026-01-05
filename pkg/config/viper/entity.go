package viper

import (
	"sync"

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
	"github.com/skolldire/go-engine/pkg/database/dynamo"
	"github.com/skolldire/go-engine/pkg/database/gormsql"
	"github.com/skolldire/go-engine/pkg/database/memcached"
	"github.com/skolldire/go-engine/pkg/database/mongodb"
	"github.com/skolldire/go-engine/pkg/database/redis"
	grpcServer "github.com/skolldire/go-engine/pkg/server/grpc"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/telemetry"
)

type Service interface {
	Apply() (Config, error)
	ApplyDynamic(log logger.Service) (*dynamic.DynamicConfig, error)
}

type Config struct {
	Router           router.Config                  `mapstructure:"router"`
	GrpcServer       *grpcServer.Config             `mapstructure:"grpc_server"`
	Rest             []map[string]rest.Config       `mapstructure:"rest"`
	GrpcClient       []map[string]grpcClient.Config `mapstructure:"grpc_client"`
	Log              logger.Config                  `mapstructure:"log"`
	Telemetry        *telemetry.Config              `mapstructure:"telemetry"`
	Aws              AwsConfig                      `mapstructure:"aws"`
	SQS              *sqs.Config                    `mapstructure:"sqs"`
	SNS              *sns.Config                    `mapstructure:"sns"`
	DataBaseSql      *gormsql.Config                `mapstructure:"database_sql"`
	Dynamo           *dynamo.Config                 `mapstructure:"dynamo"`
	Redis            *redis.Config                  `mapstructure:"redis"`
	SQSClients       []map[string]sqs.Config        `mapstructure:"sqs_clients"`
	SNSClients       []map[string]sns.Config        `mapstructure:"sns_clients"`
	DynamoClients    []map[string]dynamo.Config     `mapstructure:"dynamo_clients"`
	RedisClients     []map[string]redis.Config      `mapstructure:"redis_clients"`
	SQLConnections   []map[string]gormsql.Config    `mapstructure:"sql_connections"`
	SSMClients       []map[string]ssm.Config        `mapstructure:"ssm_clients"`
	SESClients       []map[string]ses.Config        `mapstructure:"ses_clients"`
	S3Clients        []map[string]s3.Config         `mapstructure:"s3_clients"`
	MemcachedClients []map[string]memcached.Config  `mapstructure:"memcached_clients"`
	MongoDBClients   []map[string]mongodb.Config    `mapstructure:"mongodb_clients"`
	RabbitMQClients  []map[string]rabbitmq.Config   `mapstructure:"rabbitmq_clients"`
	Repositories     map[string]interface{}         `mapstructure:"repositories"`
	Cases            map[string]interface{}         `mapstructure:"cases"`
	Endpoints        map[string]interface{}         `mapstructure:"endpoints"`
	Processors       map[string]interface{}         `mapstructure:"processors"`
	Middleware       map[string]interface{}         `mapstructure:"middleware"`
	GracefulShutdown *GracefulShutdownConfig        `mapstructure:"graceful_shutdown"`
	FeatureFlags     map[string]interface{}         `mapstructure:"feature_flags"`
}

type AwsConfig struct {
	Region string `json:"region"`
}

type service struct {
	propertyFiles []string
	path          string
	log           *logrus.Logger
}

type GracefulShutdownConfig struct {
	Timeout int `mapstructure:"timeout_seconds"`
}

var (
	instance Service
	once     sync.Once
)
