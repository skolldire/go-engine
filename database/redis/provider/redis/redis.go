// Package redis contributes a Redis client to an engine.
package redis

import (
	"context"

	goredis "github.com/skolldire/go-engine/database/redis/pkg/database/redis"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "redis_clients"

// Provider builds one named Redis client.
type Provider struct {
	instance string
	client   *goredis.RedisClient
}

var _ engine.Provider = (*Provider)(nil)

// New returns a provider for the Redis client declared under instance.
func New(instance string) *Provider {
	return &Provider{instance: instance}
}

// Name implements engine.Provider.
func (p *Provider) Name() string { return "redis:" + p.instance }

// ConfigKey implements engine.Provider.
func (p *Provider) ConfigKey() string { return ConfigKey }

// Init implements engine.Provider. It also contributes a health check, so a
// Redis outage surfaces on /ready without the application wiring anything.
func (p *Provider) Init(_ context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
	cfg, err := engine.DecodeNamed[goredis.Config](raw, p.instance)
	if err != nil {
		return nil, err
	}

	client, err := goredis.NewClient(cfg, deps.Logger)
	if err != nil {
		return nil, err
	}
	p.client = client

	deps.Health.RegisterCheck(p.Name(), func(ctx context.Context) error {
		return client.Ping(ctx)
	})

	return client, nil
}

// Close implements engine.Provider.
func (p *Provider) Close(context.Context) error {
	if p.client == nil {
		return nil
	}
	return p.client.Close()
}

// From retrieves the Redis client registered under instance.
func From(e *engine.Engine, instance string) (*goredis.RedisClient, error) {
	return engine.Get[*goredis.RedisClient](e, "redis:"+instance)
}
