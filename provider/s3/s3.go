// Package s3 contributes a S3 client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package s3

import (
	"context"
	"fmt"

	awss3 "github.com/skolldire/go-engine/aws/pkg/clients/s3"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/provider/awsbase"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "s3_clients"

// Provider builds one named S3 client.
type Provider struct {
	instance string
	client   awss3.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the S3 client declared under instance.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "s3:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[awss3.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	log, ok := deps.Logger.(logger.Service)
	if !ok {
		return nil, fmt.Errorf("s3 requires a logger.Service, got %T", deps.Logger)
	}

	awsCfg, err := awsbase.Config(ctx, deps)
	if err != nil {
		return nil, err
	}

	p.client = awss3.NewClient(awsCfg, cfg, log)

	return p.client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error {
	// This client owns no connection of its own; the AWS SDK transport is
	// shared and released with the process.
	return nil
}

// From retrieves the S3 client registered under instance.
func From(e *engine.Engine, instance string) (awss3.Service, error) {
	return engine.Get[awss3.Service](e, "s3:"+instance)
}
