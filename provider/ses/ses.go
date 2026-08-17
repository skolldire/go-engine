// Package ses contributes a SES client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package ses

import (
	"context"
	"fmt"

	awsses "github.com/skolldire/go-engine/aws/pkg/clients/ses"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/provider/awsbase"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "ses_clients"

// Provider builds one named SES client.
type Provider struct {
	instance string
	client   awsses.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the SES client declared under instance.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "ses:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[awsses.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	log, ok := deps.Logger.(logger.Service)
	if !ok {
		return nil, fmt.Errorf("ses requires a logger.Service, got %T", deps.Logger)
	}

	awsCfg, err := awsbase.Config(ctx, deps)
	if err != nil {
		return nil, err
	}

	p.client = awsses.NewClient(awsCfg, cfg, log)

	return p.client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error {
	// This client owns no connection of its own; the AWS SDK transport is
	// shared and released with the process.
	return nil
}

// From retrieves the SES client registered under instance.
func From(e *engine.Engine, instance string) (awsses.Service, error) {
	return engine.Get[awsses.Service](e, "ses:"+instance)
}
