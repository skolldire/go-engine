package grpc

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"google.golang.org/grpc"
)

// Service is the public interface for the go-engine gRPC server.
type Service interface {
	// RegisterService calls registerFunc with the underlying *grpc.Server so
	// that the caller can register one or more generated service implementations.
	// It must be called before Start.
	//
	// Example:
	//
	//   srv.RegisterService(func(s *grpc.Server) {
	//       pb.RegisterAssessmentServiceServer(s, &myImpl{})
	//   })
	RegisterService(registerFunc func(server *grpc.Server))

	// Start begins listening on the configured port and serves incoming RPCs in
	// a background goroutine. It is non-blocking: control returns to the caller
	// as soon as the listener is bound.
	//
	// When ctx is cancelled, Start triggers a GracefulStop that allows in-flight
	// RPCs to complete before the listener is closed.
	Start(ctx context.Context) error

	// Stop immediately triggers GracefulStop on the underlying *grpc.Server.
	// It is safe to call multiple times and safe to call if Start was never called.
	Stop()
}

// Config holds the configuration for the gRPC server.
//
// Note: the port field is named Puerto (Spanish) for historical reasons.
// In the application YAML it maps to the key "puerto".
type Config struct {
	// Puerto is the TCP port the server listens on (e.g. 50051).
	// Maps to the YAML key "puerto".
	Puerto int `mapstructure:"puerto" json:"puerto"`

	// EnableLogging controls whether the server emits Info log entries on
	// start and stop events.
	EnableLogging bool `mapstructure:"enable_logging" json:"enable_logging"`
}

type server struct {
	server  *grpc.Server
	puerto  int
	logger  logger.Service
	logging bool
}
