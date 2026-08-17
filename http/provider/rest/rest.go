// Package rest contributes a REST client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package rest

import (
	"context"

	gorest "github.com/skolldire/go-engine/http/pkg/rest"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "rest"

// Provider builds one named REST client.
type Provider struct {
	instance string
	client   gorest.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the REST client declared under instance.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "rest:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(_ context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[gorest.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	p.client = gorest.NewClient(cfg, deps.Logger)

	return p.client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error {
	// This client owns no connection of its own; the AWS SDK transport is
	// shared and released with the process.
	return nil
}

// From retrieves the REST client registered under instance.
func From(e *engine.Engine, instance string) (gorest.Service, error) {
	return engine.Get[gorest.Service](e, "rest:"+instance)
}
