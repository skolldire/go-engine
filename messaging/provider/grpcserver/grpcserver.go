// Package grpcserver contributes a gRPC server to an engine.
package grpcserver

import (
	"context"
	"fmt"

	grpcsrv "github.com/skolldire/go-engine/messaging/pkg/server/grpc"
	"github.com/skolldire/go-engine/pkg/engine"
	"google.golang.org/grpc"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "grpc_server"

// Provider builds the gRPC server.
type Provider struct {
	opts   []grpc.ServerOption
	server grpcsrv.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns the gRPC server provider.
//
// opts reach grpc.NewServer untouched. Interceptors must be supplied here
// because gRPC fixes them when the server is constructed, and the provider is
// what constructs it:
//
//	engine.WithProvider(grpcserver.New(
//	    grpc.ChainUnaryInterceptor(authInterceptor, tracingInterceptor),
//	))
func New(opts ...grpc.ServerOption) *Provider { return &Provider{opts: opts} }

// Name implements engine.Provider.
func (p *Provider) Name() string { return "grpc_server" }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	if !raw.Exists() {
		return nil, fmt.Errorf("no %q section declared", ConfigKey)
	}

	var cfg grpcsrv.Config
	if err := raw.Decode(&cfg); err != nil {
		return nil, err
	}

	p.server = grpcsrv.NewServer(ctx, cfg, deps.Logger, p.opts...)
	return p.server, nil
}

// Close implements engine.Provider. Stop is synchronous and lets in-flight RPCs
// finish, so the engine's LIFO shutdown drains the server before releasing the
// clients it depends on.
func (p *Provider) Close(ctx context.Context) error {
	if p.server == nil {
		return nil
	}
	// The engine closes under a deadline; passing it on is what stops a hung
	// RPC from holding shutdown open indefinitely. A forced drain is returned
	// rather than swallowed, so it surfaces in the aggregated shutdown error.
	return p.server.Stop(ctx)
}

// From retrieves the gRPC server.
func From(e *engine.Engine) (grpcsrv.Service, error) {
	return engine.Get[grpcsrv.Service](e, "grpc_server")
}
