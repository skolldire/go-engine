package sqs

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

type Service interface {
	SendMessage(ctx context.Context, queueURL string, mensaje string, atributos map[string]types.MessageAttributeValue) (string, error)
	SendJSON(ctx context.Context, queueURL string, mensaje any, atributos map[string]types.MessageAttributeValue) (string, error)
	ReceiveMessages(ctx context.Context, queueURL string, maxMensajes int32, tiempoEspera int32) ([]types.Message, error)
	DeleteMessage(ctx context.Context, queueURL string, receiptHandle string) error
	CreateQueue(ctx context.Context, nombre string, atributos map[string]string) (string, error)
	DeleteQueue(ctx context.Context, queueURL string) error
	ListQueue(ctx context.Context, prefijo string) ([]string, error)
	GetURLQueue(ctx context.Context, nombre string) (string, error)
	EnableLogging(activar bool)
}

type Config struct {
	Endpoint       string            `mapstructure:"endpoint" json:"endpoint"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
}

var (
	ErrSendMessage     = errors.New("error sending message")
	ErrReceiveMessages = errors.New("error receiving messages")
	ErrDeleteMessage   = errors.New("error deleting message")
	ErrCreateQueue     = errors.New("error creating queue")
	ErrDeleteQueue     = errors.New("error deleting queue")
	ErrListQueues      = errors.New("error listing queues")
	ErrGetQueueURL     = errors.New("error getting queue URL")
	ErrInvalidInput    = errors.New("invalid input")
)

const (
	DefaultTimeout = 5 * time.Second
)

type Cliente struct {
	cliente    *sqs.Client
	logger     logger.Service
	logging    bool
	resilience *resilience.Service
}
