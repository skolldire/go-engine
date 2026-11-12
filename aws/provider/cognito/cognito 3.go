// Package cognito contributes an AWS Cognito client to an engine.
package cognito

import (
	"context"
	"fmt"

	awscognito "github.com/skolldire/go-engine/aws/pkg/clients/cognito"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes. Cognito is
// single-instance: an application authenticates against one user pool.
const ConfigKey = "cognito"

// Provider builds the Cognito client.
type Provider struct {
	client awscognito.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns the Cognito provider.
func New() *Provider { return &Provider{} }

// Name implements engine.Provider.
func (p *Provider) Name() string { return "cognito" }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	if !raw.Exists() {
		return nil, fmt.Errorf("no %q section declared", ConfigKey)
	}

	var cfg awscognito.Config
	if err := raw.Decode(&cfg); err != nil {
		return nil, err
	}

	client, err := awscognito.NewClient(ctx, cfg, deps.Logger)
	if err != nil {
		return nil, err
	}
	p.client = client
	return client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error { return nil }

// From retrieves the Cognito client.
func From(e *engine.Engine) (awscognito.Service, error) {
	return engine.Get[awscognito.Service](e, "cognito")
}
