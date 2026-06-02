package app

import (
	"github.com/skolldire/go-engine/aws/pkg/clients/s3"
	"github.com/skolldire/go-engine/aws/pkg/clients/ses"
	"github.com/skolldire/go-engine/aws/pkg/clients/sns"
	"github.com/skolldire/go-engine/aws/pkg/clients/sqs"
	"github.com/skolldire/go-engine/aws/pkg/clients/ssm"
	"github.com/skolldire/go-engine/aws/pkg/database/dynamo"
	"github.com/skolldire/go-engine/database/memcached/pkg/database/memcached"
	"github.com/skolldire/go-engine/database/mongodb/pkg/database/mongodb"
	"github.com/skolldire/go-engine/database/redis/pkg/database/redis"
	grpcClient "github.com/skolldire/go-engine/messaging/pkg/integration/grpc"
	"github.com/skolldire/go-engine/messaging/pkg/integration/rabbitmq"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	"github.com/skolldire/go-engine/pkg/health"
	pkgotel "github.com/skolldire/go-engine/pkg/telemetry/otel"
)

// ServiceRegistry holds all service clients in organized groups
// This reduces the number of fields in Engine struct
type ServiceRegistry struct {
	// HTTP clients
	RESTClients map[string]rest.Service

	// gRPC clients
	GRPCClients map[string]grpcClient.Service

	// Message queue clients
	SQSClients      map[string]sqs.Service
	SNSClients      map[string]sns.Service
	RabbitMQClients map[string]rabbitmq.Service

	// Database clients
	DynamoDBClients  map[string]dynamo.Service
	RedisClients     map[string]*redis.RedisClient
	MemcachedClients map[string]memcached.Service
	MongoDBClients   map[string]mongodb.Service

	// AWS service clients
	S3Clients  map[string]s3.Service
	SESClients map[string]ses.Service
	SSMClients map[string]ssm.Service

	// Custom clients - generic storage for any custom client implementations
	CustomClients map[string]interface{}

	// Health service
	Health *health.HealthService

	// OTel provider
	OTELProvider pkgotel.Provider
}

// NewServiceRegistry creates a new empty service registry
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		RESTClients:      make(map[string]rest.Service),
		GRPCClients:      make(map[string]grpcClient.Service),
		SQSClients:       make(map[string]sqs.Service),
		SNSClients:       make(map[string]sns.Service),
		RabbitMQClients:  make(map[string]rabbitmq.Service),
		DynamoDBClients:  make(map[string]dynamo.Service),
		RedisClients:     make(map[string]*redis.RedisClient),
		MemcachedClients: make(map[string]memcached.Service),
		MongoDBClients:   make(map[string]mongodb.Service),
		S3Clients:        make(map[string]s3.Service),
		SESClients:       make(map[string]ses.Service),
		SSMClients:       make(map[string]ssm.Service),
		CustomClients:    make(map[string]interface{}),
	}
}

// ConfigRegistry holds configuration maps
type ConfigRegistry struct {
	Repositories map[string]interface{}
	UseCases     map[string]interface{}
	Handlers     map[string]interface{}
	Batches      map[string]interface{}
}

// NewConfigRegistry creates a new empty config registry
func NewConfigRegistry() *ConfigRegistry {
	return &ConfigRegistry{
		Repositories: make(map[string]interface{}),
		UseCases:     make(map[string]interface{}),
		Handlers:     make(map[string]interface{}),
		Batches:      make(map[string]interface{}),
	}
}
