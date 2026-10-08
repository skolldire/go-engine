// Package full is the preset that reproduces the historical all-in-one engine.
//
// It registers every provider the configuration file declares, across every
// adapter family. Importing it therefore pulls in the whole dependency set on
// purpose — that is the trade this preset exists to make. An application that
// wants a smaller footprint imports the individual providers, or a narrower
// preset such as preset/http or preset/aws.
package full

import (
	awspreset "github.com/skolldire/go-engine/aws/preset"
	"github.com/skolldire/go-engine/aws/provider/cognito"
	"github.com/skolldire/go-engine/database/memcached/provider/memcached"
	"github.com/skolldire/go-engine/database/mongodb/provider/mongodb"
	"github.com/skolldire/go-engine/database/redis/provider/redis"
	"github.com/skolldire/go-engine/http/provider/rest"
	"github.com/skolldire/go-engine/messaging/provider/grpcclient"
	"github.com/skolldire/go-engine/messaging/provider/grpcserver"
	"github.com/skolldire/go-engine/messaging/provider/kafka"
	"github.com/skolldire/go-engine/messaging/provider/rabbitmq"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/provider/otel"
)

// Options returns the options for an engine with the router, health probes and
// every component declared in the configuration file.
//
//	eng, err := engine.New(ctx, full.Options()...)
func Options() []engine.Option {
	return []engine.Option{
		engine.WithRouter(),
		engine.WithHealth(),
		engine.WithProviderFunc(Providers),
	}
}

// Providers returns one provider per component declared in cfg.
func Providers(cfg *engine.Config) ([]engine.Provider, error) {
	out, err := awspreset.Providers(cfg)
	if err != nil {
		return nil, err
	}

	families := []struct {
		key string
		new func(string) engine.Provider
	}{
		{redis.ConfigKey, func(n string) engine.Provider { return redis.New(n) }},
		{memcached.ConfigKey, func(n string) engine.Provider { return memcached.New(n) }},
		{mongodb.ConfigKey, func(n string) engine.Provider { return mongodb.New(n) }},
		{rabbitmq.ConfigKey, func(n string) engine.Provider { return rabbitmq.New(n) }},
		{rest.ConfigKey, func(n string) engine.Provider { return rest.New(n) }},
		{grpcclient.ConfigKey, func(n string) engine.Provider { return grpcclient.New(n) }},
	}

	for _, f := range families {
		if err := engine.EachInstance(cfg, f.key, func(name string) {
			out = append(out, f.new(name))
		}); err != nil {
			return nil, err
		}
	}

	// Single-instance components: registered only when declared.
	singles := []struct {
		key string
		new func() engine.Provider
	}{
		{cognito.ConfigKey, func() engine.Provider { return cognito.New() }},
		{kafka.ConfigKey, func() engine.Provider { return kafka.New() }},
		{grpcserver.ConfigKey, func() engine.Provider { return grpcserver.New() }},
		{otel.ConfigKey, func() engine.Provider { return otel.New() }},
	}

	for _, sgl := range singles {
		if cfg.Section(sgl.key).Exists() {
			out = append(out, sgl.new())
		}
	}

	return out, nil
}
