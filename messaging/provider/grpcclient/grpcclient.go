// Package grpcclient contributes a GRPC client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package grpcclient

import (
	"context"

	gogrpc "github.com/skolldire/go-engine/messaging/pkg/integration/grpc"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "grpc_client"

// Provider builds one named GRPC client.
type Provider struct {
	instance string
	client   gogrpc.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the GRPC client declared under instance.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "grpc:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(_ context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[gogrpc.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	client, err := gogrpc.NewClient(cfg, deps.Logger)
	if err != nil {
		return nil, err
	}
	p.client = client

	return p.client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(ctx context.Context) error {
	if p.client == nil {
		return nil
	}
	return p.client.Close()
}

// From retrieves the GRPC client registered under instance.
func From(e *engine.Engine, instance string) (gogrpc.Service, error) {
	return engine.Get[gogrpc.Service](e, "grpc:"+instance)
}
