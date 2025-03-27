package dynamo

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
)

const (
	DefaultTimeout         = 30 * time.Second
	DefaultQueryLimit      = int32(50)
	DefaultMaxBatchItems   = 25
	DefaultItemNotFoundMsg = "ítem no encontrado"
)

var (
	ErrItemNotFound    = errors.New(DefaultItemNotFoundMsg)
	ErrInvalidKey      = errors.New("clave primaria inválida")
	ErrBatchSizeExceed = errors.New("tamaño de lote excede el máximo permitido")
	ErrMarshal         = errors.New("error al serializar datos")
	ErrUnmarshal       = errors.New("error al deserializar datos")
)

type Service interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

type Config struct {
	Endpoint          string                  `mapstructure:"endpoint"`
	TablePrefix       string                  `mapstructure:"table_prefix"`
	EnableLogging     bool                    `mapstructure:"enable_logging"`
	RetryConfig       *retry_backoff.Config   `mapstructure:"retry_config"`
	CircuitBreakerCfg *circuit_breaker.Config `mapstructure:"circuit_breaker_config"`
}

type DynamoClient struct {
	client         Service
	logger         logger.Service
	logging        bool
	retryer        *retry_backoff.Retryer
	circuitBreaker *circuit_breaker.CircuitBreaker
	tablePrefix    string
}
