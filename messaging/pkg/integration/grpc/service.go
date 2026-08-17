package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
)

func NewClient(cfg Config, log logger.Service) (Service, error) {
	timeout := cfg.TimeOut
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	creds, err := buildTransportCredentials(cfg.TLS)
	if err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}

	dialOpts := append(opts, grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
		dialer := &net.Dialer{}
		return dialer.DialContext(ctx, "tcp", s)
	}))
	conn, err := grpc.NewClient(cfg.Target, dialOpts...)
	if err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	// grpc.NewClient returns a connection in IDLE and does not start the
	// handshake until the first RPC. Connect kicks off the transport so the
	// client is usable immediately and so waitForConnection can observe progress
	// instead of blocking on a state that would never change.
	conn.Connect()

	if cfg.WaitForReady {
		if err := waitForConnection(ctx, conn); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}

	return &Cliente{
		conn:   conn,
		creds:  creds,
		target: cfg.Target,
		BaseClient: client.NewBaseClientWithName(client.BaseConfig{
			EnableLogging:  cfg.EnableLogging,
			WithResilience: cfg.WithResilience,
			Resilience:     cfg.Resilience,
			Timeout:        timeout,
		}, log, "gRPC"),
	}, nil
}

// isUsable reports whether an RPC can be issued on a connection in this state.
// CONNECTING counts: grpc queues the call until the transport is up, which is
// the normal state right after construction now that connections are lazy.
func isUsable(state connectivity.State) bool {
	return state == connectivity.Ready ||
		state == connectivity.Idle ||
		state == connectivity.Connecting
}

// waitForConnection blocks until conn reaches READY, ctx expires or the
// connection is shut down. TRANSIENT_FAILURE is retried rather than treated as
// fatal: it is the expected state while the target is still starting up.
func waitForConnection(ctx context.Context, conn *grpc.ClientConn) error {
	for {
		state := conn.GetState()

		switch state {
		case connectivity.Ready:
			return nil
		case connectivity.Shutdown:
			return fmt.Errorf("%w: connection state %v", ErrConnection, state)
		case connectivity.TransientFailure, connectivity.Idle:
			// Both states are terminal for grpc unless something asks it to
			// retry, so re-arm the transport before waiting again.
			conn.Connect()
		}

		if !conn.WaitForStateChange(ctx, state) {
			return fmt.Errorf("%w: last state %v", ErrTimeoutConnect, state)
		}
	}
}

func (c *Cliente) WithMetadata(ctx context.Context, md metadata.MD) context.Context {
	return metadata.NewOutgoingContext(ctx, md)
}

func (c *Cliente) WithHeaders(ctx context.Context, headers map[string]string) context.Context {
	md := metadata.New(headers)
	return metadata.NewOutgoingContext(ctx, md)
}

func (c *Cliente) GetConnection() *grpc.ClientConn {
	return c.conn
}

func (c *Cliente) CheckConnection() connectivity.State {
	return c.conn.GetState()
}

func (c *Cliente) Close() error {
	return c.conn.Close()
}

// WithLogging toggles logging at runtime; the BaseClient middleware reads the
// flag on every call.
func (c *Cliente) WithLogging(enable bool) {
	c.SetLogging(enable)
}

// InvokeRPC runs an RPC through the BaseClient middleware chain.
//
// There is deliberately no manual reconnection step. grpc.ClientConn already
// reconnects on its own with its own backoff; the previous ReconnectIfNeeded
// dialled a replacement connection by hand, which duplicated that logic, could
// block while holding a write lock, and threw away the healthy subchannels the
// SDK was already re-establishing. Connect() nudges an idle connection instead.
func (c *Cliente) InvokeRPC(ctx context.Context, operationName string,
	invokeFunc func(ctx context.Context) (any, error)) (any, error) {
	if !isUsable(c.conn.GetState()) {
		c.conn.Connect()
	}

	return c.Execute(ctx, operationName, invokeFunc)
}
