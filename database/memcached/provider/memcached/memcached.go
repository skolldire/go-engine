// Package memcached contributes a MEMCACHED client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package memcached

import (
	"context"

	gomemcached "github.com/skolldire/go-engine/database/memcached/pkg/database/memcached"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "memcached_clients"

// Provider builds one named MEMCACHED client.
type Provider struct {
	instance string
	client   gomemcached.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the MEMCACHED client declared under instance.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "memcached:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[gomemcached.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	client, err := gomemcached.NewClient(ctx, cfg, deps.Logger)
	if err != nil {
		return nil, err
	}
	p.client = client

	return p.client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error {
	// The memcached Service interface exposes no Close: the underlying client
	// keeps pooled connections that are released with the process.
	return nil
}

// From retrieves the MEMCACHED client registered under instance.
func From(e *engine.Engine, instance string) (gomemcached.Service, error) {
	return engine.Get[gomemcached.Service](e, "memcached:"+instance)
}
