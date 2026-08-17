// Package sns contributes a SNS client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package sns

import (
	"context"

	awssns "github.com/skolldire/go-engine/aws/pkg/clients/sns"
	"github.com/skolldire/go-engine/aws/provider/awsbase"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "sns_clients"

// Provider builds one named SNS client.
type Provider struct {
	instance string
	client   awssns.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the SNS client declared under instance.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "sns:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[awssns.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	awsCfg, err := awsbase.Config(ctx, deps)
	if err != nil {
		return nil, err
	}

	p.client = awssns.NewClient(awsCfg, cfg, deps.Logger)

	return p.client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error {
	// This client owns no connection of its own; the AWS SDK transport is
	// shared and released with the process.
	return nil
}

// From retrieves the SNS client registered under instance.
func From(e *engine.Engine, instance string) (awssns.Service, error) {
	return engine.Get[awssns.Service](e, "sns:"+instance)
}
