package grpc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"google.golang.org/grpc"
)

// ErrForcedShutdown reports that the drain deadline expired and in-flight RPCs
// were dropped, as opposed to a clean shutdown.
var ErrForcedShutdown = errors.New("gRPC server shutdown was forced")

// ErrServerStopped reports that Start was called on a server that has already
// been stopped.
//
// A *grpc.Server is single-use: once stopped it rejects every connection for
// good. Without this check Start looked like it succeeded — it bound a listener
// and returned nil — while Serve returned immediately and the server accepted
// nothing. Callers got a silently deaf server and a leaked listener. Restarting
// requires a new server, so build one with NewServer.
var ErrServerStopped = errors.New("gRPC server is stopped and cannot be restarted")

// ErrAlreadyStarted reports that Start was called on a server that is already
// serving. Without it a second Start bound a second listener and leaked it,
// since only the first is ever handed to Serve or closed.
var ErrAlreadyStarted = errors.New("gRPC server is already started")

// DefaultShutdownTimeout bounds a drain that was not given one.
const DefaultShutdownTimeout = 30 * time.Second

// Service is the public interface for the go-engine gRPC server.
type Service interface {
	// RegisterService calls registerFunc with the underlying *grpc.Server so
	// that the caller can register one or more generated service implementations.
	// It must be called before Start: gRPC rejects registrations once the
	// server is serving.
	//
	// Obtain the server from the engine, register, then start:
	//
	//   import grpcsrvprovider "github.com/skolldire/go-engine/messaging/provider/grpcserver"
	//
	//   eng, err := engine.New(ctx, engine.WithProvider(grpcsrvprovider.New()))
	//   ...
	//   srv, err := grpcsrvprovider.From(eng)
	//   ...
	//   srv.RegisterService(func(s *grpc.Server) {
	//       pb.RegisterAssessmentServiceServer(s, &myImpl{})
	//   })
	//   err = srv.Start(ctx)
	//
	// The provider builds the server from the grpc_server configuration section
	// and registers its Stop with the engine's shutdown, so the drain happens
	// in reverse dependency order without the application arranging it.
	RegisterService(registerFunc func(server *grpc.Server))

	// Start begins listening on the configured port and serves incoming RPCs in
	// a background goroutine. It is non-blocking: control returns to the caller
	// as soon as the listener is bound.
	//
	// When ctx is cancelled, Start triggers a drain bounded by
	// Config.ShutdownTimeout, allowing in-flight RPCs to complete before the
	// listener is closed.
	//
	// Start returns ErrServerStopped if the server has already been stopped: a
	// *grpc.Server is single-use and cannot be restarted. Build a new one with
	// NewServer instead. It returns ErrAlreadyStarted on a second call, rather
	// than binding a second listener nothing would ever close.
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
	// It returns ErrForcedShutdown when the deadline expired and the close had
	// to be forced.
	//
	// It is safe to call concurrently, repeatedly, and before Start. Concurrent
	// callers share one drain but are bounded individually: each returns on its
	// own ctx, so a caller asking for one second is never held by another
	// caller asking for thirty.
	//
	// Once any caller forces the close, every caller reports ErrForcedShutdown,
	// including one whose own deadline had room to spare. A forced shutdown cut
	// live connections, and returning nil to the caller that happened to be
	// waiting would hide that from the engine's aggregated shutdown error.
	//
	// Limit worth knowing: forcing closes listeners and transports, which
	// cancels every handler's context. A handler that ignores its context
	// cannot be aborted — Go cannot stop a goroutine from outside, and gRPC
	// exposes no way to abandon one. Stop therefore guarantees that the caller
	// is released on time, not that the process is free of the handler. Make
	// handlers honour their context.
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
	server          *grpc.Server
	puerto          int
	logger          logger.Service
	logging         bool
	shutdownTimeout time.Duration

	// mu guards the whole lifecycle: addr, the started/stopped flags and the
	// lazily created drained channel.
	//
	// One mutex rather than two. Start used to read the stopped flag, release
	// its lock and only then bind the listener, so a Stop landing in that gap
	// let Start return nil for a server that was already finished. Holding the
	// lock across net.Listen closes the window outright; the call is a syscall,
	// not a drain, so nothing waits on it for long. A four-state machine would
	// buy the same guarantee at more cost than the problem is worth.
	mu      sync.Mutex
	addr    string
	started bool
	stopped bool
	drained chan struct{}

	// Shutdown coordination.
	//
	// sync.Once was the wrong primitive: Do blocks every later caller until the
	// first returns, so a caller with a one-second deadline waited out another
	// caller's thirty-second drain with its own context ignored. What is wanted
	// is one drain, observed by many callers, each bounded by its own context.
	//
	//   drainOnce  starts the single graceful drain
	//   drained    closes when that drain finishes
	//   forceOnce  guards the forced close, which any caller may trigger
	//   forced     records that a force happened, for every caller to read
	drainOnce sync.Once
	forceOnce sync.Once
	forced    atomic.Bool
}
