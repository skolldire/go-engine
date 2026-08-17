// Package mongodb contributes a MONGODB client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package mongodb

import (
	"context"
	"fmt"

	gomongo "github.com/skolldire/go-engine/database/mongodb/pkg/database/mongodb"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "mongodb_clients"

// Provider builds one named MONGODB client.
type Provider struct {
	instance string
	client   gomongo.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the MONGODB client declared under instance.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "mongodb:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[gomongo.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	log, ok := deps.Logger.(logger.Service)
	if !ok {
		return nil, fmt.Errorf("mongodb requires a logger.Service, got %T", deps.Logger)
	}

	client, err := gomongo.NewClient(ctx, cfg, log)
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
	return p.client.Disconnect(ctx)
}

// From retrieves the MONGODB client registered under instance.
func From(e *engine.Engine, instance string) (gomongo.Service, error) {
	return engine.Get[gomongo.Service](e, "mongodb:"+instance)
}
