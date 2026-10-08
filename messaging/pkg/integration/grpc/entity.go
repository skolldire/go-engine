package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

const DefaultTimeout = 30 * time.Second

var (
	ErrConnection     = fmt.Errorf("error connecting to gRPC server")
	ErrTimeoutConnect = fmt.Errorf("timeout waiting for gRPC connection")
)

type Service interface {
	WithMetadata(ctx context.Context, md metadata.MD) context.Context
	WithHeaders(ctx context.Context, headers map[string]string) context.Context
	GetConnection() *grpc.ClientConn
	CheckConnection() connectivity.State
	Close() error
	WithLogging(enable bool)
	InvokeRPC(ctx context.Context, operationName string,
		invokeFunc func(ctx context.Context) (any, error)) (any, error)
}

type Config struct {
	Target         string            `mapstructure:"target" json:"target"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
	TimeOut        time.Duration     `mapstructure:"timeout" json:"timeout"`
	// TLS configures transport security. When nil or disabled the client uses an
	// insecure (plaintext) connection.
	TLS *TLSConfig `mapstructure:"tls" json:"tls"`
	// WaitForReady makes NewClient block until the connection reaches READY or
	// TimeOut elapses. It defaults to false: gRPC connections are lazy by design
	// and the first RPC establishes the transport, so blocking at construction
	// time is only useful for fail-fast startup checks.
	WaitForReady bool `mapstructure:"wait_for_ready" json:"wait_for_ready"`
}

// Cliente embeds BaseClient, so timeout handling, logging, metrics, tracing and
// resilience come from its middleware chain instead of being re-implemented
// here. That removed roughly forty lines that duplicated BaseClient.Execute.
type Cliente struct {
	conn   *grpc.ClientConn
	creds  credentials.TransportCredentials
	target string

	*client.BaseClient
}
