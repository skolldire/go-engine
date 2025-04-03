package grpc

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"google.golang.org/grpc"
)

type Service interface {
	RegisterService(registerFunc func(server *grpc.Server))
	Start(ctx context.Context) error
	Stop()
}

type Config struct {
	Puerto        int  `mapstructure:"puerto" json:"puerto"`
	EnableLogging bool `mapstructure:"enable_logging" json:"enable_logging"`
}

type server struct {
	server  *grpc.Server
	puerto  int
	logger  logger.Service
	logging bool
}
