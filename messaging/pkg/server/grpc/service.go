package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewServer creates a gRPC server with gRPC reflection enabled.
// Reflection allows tools like grpcurl and Postman to discover services without
// pre-compiled proto descriptors.
//
// Interceptors (auth, logging, tracing, etc.) must be provided via
// grpc.NewServer options before calling NewServer, or registered on the
// underlying *grpc.Server after construction via RegisterService.
//
// To add interceptors, use the AppBuilder pattern:
//
//	// in the service's main.go, after Build():
//	engine.GrpcServer.RegisterService(func(s *grpc.Server) {
//	    // s is already created; interceptors must be set at grpc.NewServer time.
//	})
//
// For interceptors, construct the server manually using google.golang.org/grpc
// directly and register it with AppBuilder.WithCustomClient.
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

// RegisterService registers one or more gRPC service implementations.
// Must be called before Start. Calling it after Start has undefined behaviour.
func (s *server) RegisterService(registerFunc func(server *grpc.Server)) {
	registerFunc(s.server)
}

// Start binds a TCP listener on the configured port and begins serving RPCs in
// a background goroutine. It returns immediately after the listener is bound.
//
// When ctx is cancelled, a separate goroutine calls GracefulStop, which
// prevents new connections and waits for active RPCs to complete before
// releasing the port.
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

// Stop calls GracefulStop on the server synchronously.
// In-flight RPCs are allowed to complete before the method returns.
// Safe to call without a prior Start and safe to call multiple times.
func (s *server) Stop() {
	if s.server != nil {
		if s.logging {
			s.logger.Info(context.Background(), "stopping gRPC server", nil)
		}
		s.server.GracefulStop()
	}
}
