package awsbase

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// Client is the shape every AWS client constructor in this repository shares:
// a context, the resolved AWS config, the adapter's own config, and a logger.
//
// The context is first and is propagated from Init, so a cancelled startup
// stops construction rather than being ignored.
type Client[C, S any] func(context.Context, aws.Config, C, logger.Service) S

// Provider builds one named AWS client.
//
// It exists because six adapters had a byte-identical provider: only the two
// type names and the section key differed. Six copies meant six edits for any
// change to the pattern, and a fix applied to five of them.
//
// It is parameterised rather than generated because the variation is exactly
// two types and two strings — the case generics are for. Each adapter keeps its
// own package and its own typed From, so the call sites stay concrete.
type Provider[C, S any] struct {
	prefix    string
	configKey string
	instance  string
	build     Client[C, S]

	client S
}

// NewProvider returns a provider for the client declared under instance.
//
// prefix names the component ("sns" yields "sns:orders"); configKey is the
// section it decodes; build is the adapter's own constructor.
func NewProvider[C, S any](prefix, configKey, instance string, build Client[C, S]) *Provider[C, S] {
	return &Provider[C, S]{
		prefix:    prefix,
		configKey: configKey,
		instance:  instance,
		build:     build,
	}
}

var _ engine.Provider = (*Provider[struct{}, any])(nil)

// Name implements engine.Provider.
func (p *Provider[C, S]) Name() string { return p.prefix + ":" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider[C, S]) ConfigKey() string { return p.configKey }

// Init implements engine.Provider.
func (p *Provider[C, S]) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[C](raw, p.instance)
	if err != nil {
		return nil, err
	}

	// Resolved once per engine and shared by every AWS provider.
	awsCfg, err := Config(ctx, deps)
	if err != nil {
		return nil, err
	}

	p.client = p.build(ctx, awsCfg, cfg, deps.Logger)

	return p.client, nil
}

// Close implements engine.Provider. These clients own no connection of their
// own: the AWS SDK transport is shared and released with the process.
func (p *Provider[C, S]) Close(context.Context) error { return nil }
