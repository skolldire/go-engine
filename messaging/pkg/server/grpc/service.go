package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func NewServer(cfg Config, log logger.Service) Service {
	grpcServer := grpc.NewServer()

	reflection.Register(grpcServer)

	return &server{
		server:  grpcServer,
		puerto:  cfg.Puerto,
		logger:  log,
		logging: cfg.EnableLogging,
	}
}

func (s *server) RegisterService(registerFunc func(server *grpc.Server)) {
	registerFunc(s.server)
}

func (s *server) Start(ctx context.Context) error {
	address := fmt.Sprintf(":%d", s.puerto)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("error starting listener: %w", err)
	}

	if s.logging {
		s.logger.Info(ctx, "starting gRPC server",
			map[string]interface{}{"puerto": s.puerto})
	}

	go func() {
		if err := s.server.Serve(listener); err != nil {
			s.logger.Error(ctx, fmt.Errorf("gRPC server error: %w", err), nil)
		}
	}()

	go func() {
		<-ctx.Done()
		if s.logging {
			s.logger.Info(context.Background(), "stopping gRPC server", nil)
		}
		s.server.GracefulStop()
	}()

	return nil
}

func (s *server) Stop() {
	if s.server != nil {
		if s.logging {
			s.logger.Info(context.Background(), "stopping gRPC server", nil)
		}
		s.server.GracefulStop()
	}
}
