// Package kafka contributes a Kafka producer/consumer to an engine.
package kafka

import (
	"context"
	"fmt"

	gokafka "github.com/skolldire/go-engine/messaging/pkg/integration/kafka"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "kafka"

// Provider builds the Kafka client, which is both producer and consumer.
type Provider struct {
	client gokafka.Client
}

var _ engine.Provider = (*Provider)(nil)

// New returns the Kafka provider.
func New() *Provider { return &Provider{} }

// Name implements engine.Provider.
func (p *Provider) Name() string { return "kafka" }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider.
func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	if !raw.Exists() {
		return nil, fmt.Errorf("no %q section declared", ConfigKey)
	}

	var cfg gokafka.Config
	if err := raw.Decode(&cfg); err != nil {
		return nil, err
	}

	client, err := gokafka.NewClient(ctx, cfg, deps.Logger)
	if err != nil {
		return nil, err
	}
	p.client = client
	return client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error {
	if p.client == nil {
		return nil
	}
	return p.client.Close()
}

// From retrieves the Kafka client.
func From(e *engine.Engine) (gokafka.Client, error) {
	return engine.Get[gokafka.Client](e, "kafka")
}
