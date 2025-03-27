package sqs

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
)

type Service interface {
	EnviarMensaje(ctx context.Context, queueURL string, mensaje string, atributos map[string]types.MessageAttributeValue) (string, error)
	EnviarMensajeJSON(ctx context.Context, queueURL string, mensaje interface{}, atributos map[string]types.MessageAttributeValue) (string, error)
	RecibirMensajes(ctx context.Context, queueURL string, maxMensajes int32, tiempoEspera int32) ([]types.Message, error)
	EliminarMensaje(ctx context.Context, queueURL string, receiptHandle string) error
	CrearCola(ctx context.Context, nombre string, atributos map[string]string) (string, error)
	EliminarCola(ctx context.Context, queueURL string) error
	ListarColas(ctx context.Context, prefijo string) ([]string, error)
	ObtenerURLCola(ctx context.Context, nombre string) (string, error)
	HabilitarLogging(activar bool)
}

type Config struct {
	Endpoint          string                  `mapstructure:"endpoint"`
	EnableLogging     bool                    `mapstructure:"enable_logging"`
	RetryConfig       *retry_backoff.Config   `mapstructure:"retry_config"`
	CircuitBreakerCfg *circuit_breaker.Config `mapstructure:"circuit_breaker_config"`
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
	cliente        *sqs.Client
	logger         logger.Service
	logging        bool
	retryer        *retry_backoff.Retryer
	circuitBreaker *circuit_breaker.CircuitBreaker
}
