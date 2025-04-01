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
	SendMsj(ctx context.Context, queueURL string, mensaje string, atributos map[string]types.MessageAttributeValue) (string, error)
	SendJSON(ctx context.Context, queueURL string, mensaje interface{}, atributos map[string]types.MessageAttributeValue) (string, error)
	ReceiveMsj(ctx context.Context, queueURL string, maxMensajes int32, tiempoEspera int32) ([]types.Message, error)
	DeleteMsj(ctx context.Context, queueURL string, receiptHandle string) error
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
	ErrEnviarMensaje   = errors.New("error al enviar mensaje")
	ErrRecibirMensajes = errors.New("error al recibir mensajes")
	ErrEliminarMensaje = errors.New("error al eliminar mensaje")
	ErrCrearCola       = errors.New("error al crear cola")
	ErrEliminarCola    = errors.New("error al eliminar cola")
	ErrListarColas     = errors.New("error al listar colas")
	ErrObtenerURLCola  = errors.New("error al obtener URL de cola")
	ErrInvalidInput    = errors.New("entrada inválida")
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
