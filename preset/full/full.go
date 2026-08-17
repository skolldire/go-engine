// Package full is the preset that reproduces the historical all-in-one engine.
//
// It registers every provider the configuration file declares, across every
// adapter family. Importing it therefore pulls in the whole dependency set on
// purpose — that is the trade this preset exists to make. An application that
// wants a smaller footprint imports the individual providers, or a narrower
// preset such as preset/http or preset/aws.
package full

import (
	"github.com/skolldire/go-engine/pkg/engine"
	awspreset "github.com/skolldire/go-engine/preset/aws"
	"github.com/skolldire/go-engine/provider/cognito"
	"github.com/skolldire/go-engine/provider/grpcclient"
	"github.com/skolldire/go-engine/provider/grpcserver"
	"github.com/skolldire/go-engine/provider/kafka"
	"github.com/skolldire/go-engine/provider/memcached"
	"github.com/skolldire/go-engine/provider/mongodb"
	"github.com/skolldire/go-engine/provider/otel"
	"github.com/skolldire/go-engine/provider/rabbitmq"
	"github.com/skolldire/go-engine/provider/redis"
	"github.com/skolldire/go-engine/provider/rest"
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
func Providers(cfg *engine.Config) []engine.Provider {
	out := awspreset.Providers(cfg)

	// Multi-instance families.
	for _, name := range engine.InstanceNames(cfg.Section(redis.ConfigKey)) {
		out = append(out, redis.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(memcached.ConfigKey)) {
		out = append(out, memcached.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(mongodb.ConfigKey)) {
		out = append(out, mongodb.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(rabbitmq.ConfigKey)) {
		out = append(out, rabbitmq.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(rest.ConfigKey)) {
		out = append(out, rest.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(grpcclient.ConfigKey)) {
		out = append(out, grpcclient.New(name))
	}

	// Single-instance components: registered only when declared.
	if cfg.Section(cognito.ConfigKey).Exists() {
		out = append(out, cognito.New())
	}
	if cfg.Section(kafka.ConfigKey).Exists() {
		out = append(out, kafka.New())
	}
	if cfg.Section(grpcserver.ConfigKey).Exists() {
		out = append(out, grpcserver.New())
	}
	if cfg.Section(otel.ConfigKey).Exists() {
		out = append(out, otel.New())
	}

	return out
}
