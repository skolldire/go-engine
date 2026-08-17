// Package sqs contributes an AWS SQS client to an engine.
//
// It is a leaf: the core does not import it, so an application that never
// registers this provider never links the AWS SDK.
package sqs

import (
	"context"
	"fmt"

	awssqs "github.com/skolldire/go-engine/aws/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/provider/awsbase"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "sqs_clients"

// Provider builds one named SQS client.
type Provider struct {
	instance string
	client   awssqs.Service
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the SQS client declared under instance in the
// sqs_clients section.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "sqs:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[awssqs.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	awsCfg, err := awsbase.Config(ctx, deps)
	if err != nil {
		return nil, err
	}

	log, ok := deps.Logger.(logger.Service)
	if !ok {
		return nil, fmt.Errorf("sqs requires a logger.Service, got %T", deps.Logger)
	}

	p.client = awssqs.NewClient(awsCfg, cfg, log)
	return p.client, nil
}

// Close implements engine.Provider. The SQS client holds no connection of its
// own, so there is nothing to release.
func (p *Provider) Close(context.Context) error { return nil }

// From retrieves the SQS client registered under instance, with its concrete
// type and a compile-time guarantee about that type.
func From(e *engine.Engine, instance string) (awssqs.Service, error) {
	return engine.Get[awssqs.Service](e, "sqs:"+instance)
}
