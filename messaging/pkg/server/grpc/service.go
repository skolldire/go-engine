package grpc

import (
	"context"
	"errors"
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
// Register your service implementations after the engine has built the server:
//
//	eng, err := engine.New(ctx, engine.WithProvider(grpcserver.New()))
//	srv, err := grpcserver.From(eng)
//
//	srv.RegisterService(func(s *grpc.Server) {
//	    pb.RegisterOrdersServer(s, ordersImpl)
//	})
//
// Interceptors are the one thing that cannot be added this way: gRPC fixes them
// at grpc.NewServer time, before this constructor returns. A service that needs
// them builds its own *grpc.Server and registers it as a custom component:
//
//	engine.Get[*grpc.Server](eng, "my-grpc-server")
func NewServer(ctx context.Context, cfg Config, log logger.Service) Service {
	grpcServer := grpc.NewServer()

	reflection.Register(grpcServer)

	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = DefaultShutdownTimeout
	}

	return &server{
		server:          grpcServer,
		puerto:          cfg.Puerto,
		logger:          log,
		logging:         cfg.EnableLogging,
		shutdownTimeout: shutdownTimeout,
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
	// Refuse to restart a stopped server rather than binding a listener that
	// would never serve a request. See ErrServerStopped.
	s.stopMu.Lock()
	stopped := s.stopped
	s.stopMu.Unlock()

	if stopped {
		return ErrServerStopped
	}

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
		// Serve returns ErrServerStopped after Stop or GracefulStop, which is
		// the normal end of a shutdown rather than a failure. Logging it as an
		// error made every clean shutdown look like an incident.
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			s.logger.Error(ctx, fmt.Errorf("gRPC server error: %w", err), nil)
		}
	}()

	go func() {
		<-ctx.Done()

		// Reuse the bounded Stop rather than calling GracefulStop directly.
		// This path had the very defect Stop was written to remove: a stuck RPC
		// left this goroutine waiting for the life of the process, and the
		// explicit shutdown path being correct did not help an application that
		// only cancels its context.
		//
		// The context is detached from the cancelled one so the drain gets its
		// full window instead of expiring immediately.
		shutdownCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx), s.shutdownTimeout)
		defer cancel()

		if err := s.Stop(shutdownCtx); err != nil && s.logging {
			s.logger.Error(context.WithoutCancel(ctx), err, nil)
		}
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

	done := s.beginDrain(ctx)

	select {
	case <-done:
		// The graceful drain finished within this caller's window.
		return nil

	case <-ctx.Done():
		// This caller's deadline expired. Force the close so the drain ends for
		// everyone, then report the forcing to this caller. Another caller with
		// a longer deadline simply observes the drain completing.
		s.force()

		// Deliberately not waiting for the drain goroutine. GracefulStop only
		// returns once every handler has returned, so waiting would let a
		// handler that never returns block shutdown for good — the exact thing
		// this deadline exists to prevent.
		return fmt.Errorf("%w: %w", ErrForcedShutdown, ctx.Err())
	}
}

// force closes the server the hard way, once, without blocking the caller.
//
// The goroutine is not defensive style, it is required. In grpc-go, Stop and
// GracefulStop share one implementation that holds the server mutex across
// handlersWG.Wait():
//
//	s.mu.Lock()
//	defer s.mu.Unlock()
//	...
//	if graceful || s.opts.waitForHandlers {
//	    s.handlersWG.Wait()
//	}
//
// So once the graceful drain is waiting on a handler, a concurrent Stop blocks
// acquiring that same mutex. Calling it inline would make the forced path hang
// on exactly the stuck handler the deadline exists to escape — the canonical
// "GracefulStop, then Stop on timeout" snippet has this bug.
//
// What this buys, and what it does not: forcing closes the listeners and the
// transports, so handlers that honour their context are cancelled and return.
// A handler that ignores its context cannot be aborted — Go has no way to stop
// a goroutine from outside — so the server keeps its resources until that
// handler returns. This method guarantees the caller is released on time; it
// cannot guarantee the process is free of the handler.
func (s *server) force() {
	s.forceOnce.Do(func() { go s.server.Stop() })
}

// beginDrain starts the graceful drain if it is not already running and returns
// the channel that closes when it finishes.
//
// The drain itself takes no context: it belongs to the server, not to whichever
// caller happened to start it. Deadlines are applied per caller in Stop.
func (s *server) beginDrain(ctx context.Context) <-chan struct{} {
	s.stopMu.Lock()
	if s.drained == nil {
		s.drained = make(chan struct{})
	}
	done := s.drained
	s.stopped = true
	s.stopMu.Unlock()

	s.drainOnce.Do(func() {
		if s.logging {
			s.logger.Info(context.WithoutCancel(ctx), "stopping gRPC server", nil)
		}

		go func() {
			defer close(done)
			s.server.GracefulStop()
		}()
	})

	return done
}

// Address implements Service.
func (s *server) Address() string {
	s.addrMu.RLock()
	defer s.addrMu.RUnlock()
	return s.addr
}
