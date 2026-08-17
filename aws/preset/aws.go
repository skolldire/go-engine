// Package aws is the preset for an AWS-backed service.
//
// It registers every AWS client the configuration file declares, and nothing
// else: no Kafka, no Mongo, no Redis. Instances are discovered from the file,
// so adding a queue to the YAML needs no code change.
package aws

import (
	"github.com/skolldire/go-engine/aws/provider/dynamo"
	"github.com/skolldire/go-engine/aws/provider/s3"
	"github.com/skolldire/go-engine/aws/provider/ses"
	"github.com/skolldire/go-engine/aws/provider/sns"
	"github.com/skolldire/go-engine/aws/provider/sqs"
	"github.com/skolldire/go-engine/aws/provider/ssm"
	"github.com/skolldire/go-engine/pkg/engine"
)

// Options returns the engine options for an AWS-backed service: every AWS
// client the configuration declares, discovered at build time.
//
//	eng, err := engine.New(ctx, aws.Options()...)
func Options() []engine.Option {
	return []engine.Option{engine.WithProviderFunc(Providers)}
}

// Providers returns one provider per AWS client declared in cfg.
func Providers(cfg *engine.Config) ([]engine.Provider, error) {
	var out []engine.Provider

	families := []struct {
		key string
		new func(string) engine.Provider
	}{
		{sqs.ConfigKey, func(n string) engine.Provider { return sqs.New(n) }},
		{sns.ConfigKey, func(n string) engine.Provider { return sns.New(n) }},
		{ses.ConfigKey, func(n string) engine.Provider { return ses.New(n) }},
		{s3.ConfigKey, func(n string) engine.Provider { return s3.New(n) }},
		{ssm.ConfigKey, func(n string) engine.Provider { return ssm.New(n) }},
		{dynamo.ConfigKey, func(n string) engine.Provider { return dynamo.New(n) }},
	}

	for _, f := range families {
		if err := engine.EachInstance(cfg, f.key, func(name string) {
			out = append(out, f.new(name))
		}); err != nil {
			return nil, err
		}
	}

	return out, nil
}
