package sns

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
)

const (
	DefaultTimeout = 5 * time.Second
)

// Errores comunes
var (
	ErrPublicacion  = errors.New("error al publicar mensaje")
	ErrSuscripcion  = errors.New("error al crear suscripción")
	ErrCrearTema    = errors.New("error al crear tema")
	ErrEliminarTema = errors.New("error al eliminar tema")
	ErrListarTemas  = errors.New("error al listar temas")
	ErrInvalidInput = errors.New("entrada inválida")
)

type Config struct {
	BaseEndpoint      string                  `mapstructure:"base_endpoint"`
	EnableLogging     bool                    `mapstructure:"enable_logging"`
	RetryConfig       *retry_backoff.Config   `mapstructure:"retry_config"`
	CircuitBreakerCfg *circuit_breaker.Config `mapstructure:"circuit_breaker_config"`
}

type Service interface {
	CrearTema(ctx context.Context, nombre string, atributos map[string]string) (string, error)
	EliminarTema(ctx context.Context, arn string) error
	ListarTemas(ctx context.Context) ([]string, error)
	PublicarMensaje(ctx context.Context, temaArn string, mensaje string, atributos map[string]types.MessageAttributeValue) (string, error)
	PublicarMensajeJSON(ctx context.Context, temaArn string, mensaje interface{}, atributos map[string]types.MessageAttributeValue) (string, error)
	CrearSuscripcion(ctx context.Context, temaArn, protocolo, endpoint string) (string, error)
	EliminarSuscripcion(ctx context.Context, suscripcionArn string) error
	HabilitarLogging(activar bool)
}

type Cliente struct {
	cliente        *sns.Client
	logger         logger.Service
	logging        bool
	retryer        *retry_backoff.Retryer
	circuitBreaker *circuit_breaker.CircuitBreaker
}
