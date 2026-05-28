package viper

import (
	"sync"

	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/aws/pkg/clients/cognito"
	grpcClient "github.com/skolldire/go-engine/messaging/pkg/integration/grpc"
	"github.com/skolldire/go-engine/messaging/pkg/integration/rabbitmq"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	kafka "github.com/skolldire/go-engine/messaging/pkg/integration/kafka"
	"github.com/skolldire/go-engine/aws/pkg/clients/s3"
	"github.com/skolldire/go-engine/aws/pkg/clients/ses"
	"github.com/skolldire/go-engine/aws/pkg/clients/sns"
	"github.com/skolldire/go-engine/aws/pkg/clients/sqs"
	"github.com/skolldire/go-engine/aws/pkg/clients/ssm"
	"github.com/skolldire/go-engine/pkg/config/dynamic"
	"github.com/skolldire/go-engine/aws/pkg/database/dynamo"
	"github.com/skolldire/go-engine/database/memcached/pkg/database/memcached"
	"github.com/skolldire/go-engine/database/mongodb/pkg/database/mongodb"
	"github.com/skolldire/go-engine/database/redis/pkg/database/redis"
	grpcServer "github.com/skolldire/go-engine/messaging/pkg/server/grpc"
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
	Dynamo           *dynamo.Config                 `mapstructure:"dynamo"`
	Redis            *redis.Config                  `mapstructure:"redis"`
	SQSClients       []map[string]sqs.Config        `mapstructure:"sqs_clients"`
	SNSClients       []map[string]sns.Config        `mapstructure:"sns_clients"`
	DynamoClients    []map[string]dynamo.Config     `mapstructure:"dynamo_clients"`
	RedisClients     []map[string]redis.Config      `mapstructure:"redis_clients"`
	SSMClients       []map[string]ssm.Config        `mapstructure:"ssm_clients"`
	SESClients       []map[string]ses.Config        `mapstructure:"ses_clients"`
	S3Clients        []map[string]s3.Config         `mapstructure:"s3_clients"`
	MemcachedClients []map[string]memcached.Config  `mapstructure:"memcached_clients"`
	MongoDBClients   []map[string]mongodb.Config    `mapstructure:"mongodb_clients"`
	RabbitMQClients  []map[string]rabbitmq.Config   `mapstructure:"rabbitmq_clients"`
	Kafka            *kafka.Config                  `mapstructure:"kafka"           json:"kafka,omitempty"`
	Cognito          *cognito.Config                `mapstructure:"cognito"`
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
	log           logger.LogWriter
}

type GracefulShutdownConfig struct {
	Timeout int `mapstructure:"timeout_seconds"`
}

var (
	instance Service
	once     sync.Once
)
