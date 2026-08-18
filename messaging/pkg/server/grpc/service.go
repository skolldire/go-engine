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
func NewServer(ctx context.Context, cfg Config, log logger.Service) Service {
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

	// Record what the kernel actually gave us: with Puerto 0 the configured
	// address says nothing about where the server can be reached.
	s.addrMu.Lock()
	s.addr = listener.Addr().String()
	s.addrMu.Unlock()

	if s.logging {
		s.logger.Info(ctx, "starting gRPC server",
			map[string]any{"puerto": s.puerto})
	}

	go func() {
		if err := s.server.Serve(listener); err != nil {
			s.logger.Error(ctx, fmt.Errorf("gRPC server error: %w", err), nil)
		}
	}()

	go func() {
		<-ctx.Done()
		if s.logging {
			// ctx is already done here, so use a detached copy that keeps its
			// values (trace IDs, request scope) for the shutdown log line.
			s.logger.Info(context.WithoutCancel(ctx), "stopping gRPC server", nil)
		}
		s.server.GracefulStop()
	}()

	return nil
}

// Stop drains the server, bounded by ctx.
//
// GracefulStop waits for in-flight RPCs with no deadline of its own, so a
// single stuck stream blocks shutdown forever. That matters because the engine
// closes its components under a deadline: without a bound here, one hung RPC
// held up the entire shutdown and the process had to be killed.
//
// When ctx expires the server is stopped forcefully, which closes open
// connections. Losing an RPC that was already past its deadline is preferable
// to never shutting down.
//
// It returns an error when the drain had to be forced, so the caller can tell a
// clean shutdown from one that dropped connections. Reporting nothing made the
// two indistinguishable, and the engine aggregates closer errors precisely so a
// forced shutdown shows up somewhere.
//
// Safe to call without a prior Start and safe to call multiple times.
func (s *server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	if s.logging {
		s.logger.Info(context.WithoutCancel(ctx), "stopping gRPC server", nil)
	}

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		s.server.GracefulStop()
	}()

	select {
	case <-drained:
		return nil

	case <-ctx.Done():
		// Stop closes the connections outright, which is what unblocks
		// GracefulStop.
		s.server.Stop()

		// Deliberately not waiting for the drain goroutine here. GracefulStop
		// only returns once every handler has returned, so waiting would make
		// a handler that never returns block shutdown for good — the exact
		// thing this deadline exists to prevent. The goroutine ends when its
		// handler does; the process is on its way down either way.
		return fmt.Errorf("%w: %w", ErrForcedShutdown, ctx.Err())
	}
}

// Address implements Service.
func (s *server) Address() string {
	s.addrMu.RLock()
	defer s.addrMu.RUnlock()
	return s.addr
}
