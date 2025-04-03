package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func NewCliente(cfg Config, log logger.Service) (Service, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.TimeOut)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	dialOpts := append(opts, grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
		dialer := &net.Dialer{}
		return dialer.DialContext(ctx, "tcp", s)
	}))
	conn, err := grpc.NewClient(cfg.Target, dialOpts...)
	if err != nil {
		return nil, log.WrapError(err, ErrConnection.Error())
	}

	if err := waitForConnection(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	c := &Cliente{
		conn:    conn,
		logger:  log,
		logging: cfg.EnableLogging,
		target:  cfg.Target,
	}

	if cfg.WithResilience {
		c.resilience = resilience.NewResilienceService(cfg.Resilience, log)
	}

	if c.logging {
		c.logger.Debug(ctx, "Conexión a servidor gRPC establecida correctamente",
			map[string]interface{}{"target": cfg.Target})
	}

	return c, nil
}

func waitForConnection(ctx context.Context, conn *grpc.ClientConn) error {
	for {
		state := conn.GetState()

		if state == connectivity.Ready {
			return nil
		}

		if state == connectivity.Shutdown || state == connectivity.TransientFailure {
			return fmt.Errorf("%w: estado de conexión %v", ErrConnection, state)
		}

		if !conn.WaitForStateChange(ctx, state) {
			return fmt.Errorf("%w: última estado %v", ErrTimeoutConnect, state)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %v", ErrTimeoutConnect, ctx.Err())
		default:
		}
	}
}

func (c *Cliente) execute(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	state := c.conn.GetState()
	if state != connectivity.Ready && state != connectivity.Idle {
		if c.logging {
			c.logger.Warn(ctx, fmt.Sprintf("Estado de conexión gRPC no óptimo: %v", state),
				map[string]interface{}{"operation": operationName})
		}
	}

	logFields := map[string]interface{}{"operation": operationName}

	if c.resilience != nil {
		if c.logging {
			c.logger.Debug(ctx, fmt.Sprintf("Iniciando operación gRPC con resiliencia: %s", operationName), logFields)
		}

		result, err := c.resilience.Execute(ctx, operation)

		if err != nil && c.logging {
			c.logger.Error(ctx, fmt.Errorf("error en operación gRPC: %w", err), logFields)
		} else if c.logging {
			c.logger.Debug(ctx, fmt.Sprintf("Operación gRPC completada con resiliencia: %s", operationName), logFields)
		}

		return result, err
	}

	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Iniciando operación gRPC: %s", operationName), logFields)
	}

	result, err := operation()

	if err != nil && c.logging {
		c.logger.Error(ctx, err, logFields)
	} else if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("Operación gRPC completada: %s", operationName), logFields)
	}

	return result, err
}

func (c *Cliente) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		timeout := time.Until(deadline)
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithTimeout(ctx, DefaultTimeout)
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

func (c *Cliente) ReconnectIfNeeded(ctx context.Context) error {
	state := c.conn.GetState()

	if state == connectivity.Ready || state == connectivity.Idle {
		return nil // La conexión está en buen estado
	}

	if c.logging {
		c.logger.Warn(ctx, "Intentando reconexión gRPC",
			map[string]interface{}{"state": state, "target": c.target})
	}

	_ = c.conn.Close()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	conn, err := grpc.NewClient(c.target, opts...)
	if err != nil {
		return c.logger.WrapError(err, ErrConnection.Error())
	}

	if err := waitForConnection(ctx, conn); err != nil {
		_ = conn.Close()
		return err
	}

	c.conn = conn

	if c.logging {
		c.logger.Info(ctx, "Reconexión gRPC exitosa",
			map[string]interface{}{"target": c.target})
	}

	return nil
}

func (c *Cliente) Close() error {
	if c.logging {
		c.logger.Debug(context.Background(), "Cerrando conexión gRPC",
			map[string]interface{}{"target": c.target})
	}
	return c.conn.Close()
}

func (c *Cliente) WithLogging(enable bool) {
	c.logging = enable
}

func (c *Cliente) InvokeRPC(ctx context.Context, operationName string,
	invokeFunc func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	state := c.conn.GetState()
	if state != connectivity.Ready && state != connectivity.Idle {
		if err := c.ReconnectIfNeeded(ctx); err != nil && c.logging {
			c.logger.Warn(ctx, "No se pudo reconectar, intentando operación con la conexión actual",
				map[string]interface{}{"error": err.Error(), "operation": operationName})
		}
	}

	return c.execute(ctx, operationName, func() (interface{}, error) {
		return invokeFunc(ctx)
	})
}
