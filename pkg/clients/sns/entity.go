package sns

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout = 5 * time.Second
)

// Errores comunes
var (
	ErrPublication  = errors.New("error al publicar mensaje")
	ErrSubscription = errors.New("error al crear suscripción")
	ErrCreateTopic  = errors.New("error al crear tema")
	ErrDeleteTopic  = errors.New("error al eliminar tema")
	ErrListTopics   = errors.New("error al listar temas")
	ErrInvalidInput = errors.New("entrada inválida")
)

type Config struct {
	BaseEndpoint   string            `mapstructure:"base_endpoint" json:"base_endpoint"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
}

type Service interface {
	CreateTopic(ctx context.Context, name string, atributos map[string]string) (string, error)
	DeleteTopic(ctx context.Context, arn string) error
	GetTopics(ctx context.Context) ([]string, error)
	PublishMsj(ctx context.Context, temaArn string, msj string, atributos map[string]types.MessageAttributeValue) (string, error)
	PublishJSON(ctx context.Context, temaArn string, msj interface{}, atributos map[string]types.MessageAttributeValue) (string, error)
	CreateSubscription(ctx context.Context, temaArn, protocolo, endpoint string) (string, error)
	DeleteSubscription(ctx context.Context, subscriptionArn string) error
	EnableLogging(activar bool)
}

type Cliente struct {
	cliente    *sns.Client
	logger     logger.Service
	logging    bool
	resilience *resilience.Service
}
