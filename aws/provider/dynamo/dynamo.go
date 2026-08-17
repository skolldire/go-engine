// Package dynamo contributes a DYNAMO client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package dynamo

import (
	"context"

	awsdynamo "github.com/skolldire/go-engine/aws/pkg/database/dynamo"
	"github.com/skolldire/go-engine/aws/provider/awsbase"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "dynamo_clients"

// Provider builds one named DYNAMO client.
type Provider struct {
	instance string
	client   awsdynamo.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the DYNAMO client declared under instance.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "dynamo:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[awsdynamo.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	awsCfg, err := awsbase.Config(ctx, deps)
	if err != nil {
		return nil, err
	}

	p.client = awsdynamo.NewClient(awsCfg, cfg, deps.Logger)

	return p.client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error {
	// This client owns no connection of its own; the AWS SDK transport is
	// shared and released with the process.
	return nil
}

// From retrieves the DYNAMO client registered under instance.
func From(e *engine.Engine, instance string) (awsdynamo.Service, error) {
	return engine.Get[awsdynamo.Service](e, "dynamo:"+instance)
}
