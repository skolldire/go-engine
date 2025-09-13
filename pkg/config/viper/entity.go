package viper

import (
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/app/router"
	grpcClient "github.com/skolldire/go-engine/pkg/clients/grpc"
	"github.com/skolldire/go-engine/pkg/clients/rest/advanced"
	"github.com/skolldire/go-engine/pkg/clients/sns"
	"github.com/skolldire/go-engine/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/database/dynamo"
	"github.com/skolldire/go-engine/pkg/database/gormsql"
	"github.com/skolldire/go-engine/pkg/database/redis"
	grpcServer "github.com/skolldire/go-engine/pkg/server/grpc"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/telemetry"
)

type Service interface {
	Apply() (Config, error)
}

type Config struct {
	Router           router.Config                  `mapstructure:"router"`
	GrpcServer       *grpcServer.Config             `mapstructure:"grpc_server"`
	Rest             []map[string]advanced.Config   `mapstructure:"rest"`
	GrpcClient       []map[string]grpcClient.Config `mapstructure:"grpc_client"`
	Log              logger.Config                  `mapstructure:"log"`
	Telemetry        *telemetry.Config              `mapstructure:"telemetry"`
	Aws              AwsConfig                      `mapstructure:"aws"`
	SQS              *sqs.Config                    `mapstructure:"sqs"`
	SNS              *sns.Config                    `mapstructure:"sns"`
	DataBaseSql      *gormsql.Config                `mapstructure:"database_sql"`
	Dynamo           *dynamo.Config                 `mapstructure:"dynamo"`
	Redis            *redis.Config                  `mapstructure:"redis"`
	Repositories     map[string]interface{}         `mapstructure:"repositories"`
	Cases            map[string]interface{}         `mapstructure:"cases"`
	Endpoints        map[string]interface{}         `mapstructure:"endpoints"`
	Processors       map[string]interface{}         `mapstructure:"processors"`
	Middleware       map[string]interface{}         `mapstructure:"middleware"`
	GracefulShutdown *GracefulShutdownConfig        `mapstructure:"graceful_shutdown"`
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
