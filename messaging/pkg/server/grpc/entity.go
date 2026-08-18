package grpc

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"google.golang.org/grpc"
)

// ErrForcedShutdown reports that the drain deadline expired and in-flight RPCs
// were dropped, as opposed to a clean shutdown.
var ErrForcedShutdown = errors.New("gRPC server shutdown was forced")

// DefaultShutdownTimeout bounds a drain that was not given one.
const DefaultShutdownTimeout = 30 * time.Second

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

	// Address reports the address the listener bound, e.g. "[::]:50051".
	// It is empty until Start has bound the port.
	//
	// It matters because Puerto may be 0, which asks the kernel for any free
	// port: without this the caller has no way to learn which one it got, so a
	// server configured that way could be started but never reached.
	Address() string

	// Stop drains the server, bounded by ctx: in-flight RPCs are allowed to
	// finish, but a stuck one cannot hold shutdown open past the deadline.
	// It returns ErrForcedShutdown when the deadline expired and connections
	// had to be dropped.
	// It is safe to call multiple times and safe to call if Start was never called.
	Stop(ctx context.Context) error
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

	// ShutdownTimeout bounds the drain started when the context passed to Start
	// is cancelled. Defaults to DefaultShutdownTimeout.
	//
	// It exists because GracefulStop waits for in-flight RPCs with no deadline
	// of its own: a single stuck stream would otherwise leave that goroutine
	// waiting for the life of the process.
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" json:"shutdown_timeout"`
}

type server struct {
	// addr is written once by Start and read by Address, so it is guarded.
	addrMu sync.RWMutex
	addr   string

	server          *grpc.Server
	puerto          int
	logger          logger.Service
	logging         bool
	shutdownTimeout time.Duration

	// stopOnce serialises every shutdown path — the explicit Stop and the one
	// the cancelled Start context triggers — so the two cannot run concurrently
	// and every caller observes the same outcome. The interface promises Stop is
	// safe to call repeatedly; this is what makes that guarantee ours rather
	// than a property of the gRPC implementation we happen to rely on.
	stopOnce sync.Once
	stopErr  error
}
