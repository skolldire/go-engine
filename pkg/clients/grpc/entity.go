package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
)

const DefaultTimeout = 30 * time.Second

var (
	ErrConnection     = fmt.Errorf("error al conectar con el servidor gRPC")
	ErrTimeoutConnect = fmt.Errorf("tiempo de espera agotado al esperar la conexión gRPC")
)

type Service interface {
	WithMetadata(ctx context.Context, md metadata.MD) context.Context
	WithHeaders(ctx context.Context, headers map[string]string) context.Context
	GetConnection() *grpc.ClientConn
	CheckConnection() connectivity.State
	ReconnectIfNeeded(ctx context.Context) error
	Close() error
	WithLogging(enable bool)
	InvokeRPC(ctx context.Context, operationName string,
		invokeFunc func(ctx context.Context) (interface{}, error)) (interface{}, error)
}

type Config struct {
	Target         string            `mapstructure:"target" json:"target"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
	TimeOut        time.Duration     `mapstructure:"timeout" json:"timeout"`
}

type Cliente struct {
	conn       *grpc.ClientConn
	logger     logger.Service
	logging    bool
	resilience *resilience.Service
	target     string
}
