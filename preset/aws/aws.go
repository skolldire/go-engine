// Package aws is the preset for an AWS-backed service.
//
// It registers every AWS client the configuration file declares, and nothing
// else: no Kafka, no Mongo, no Redis. Instances are discovered from the file,
// so adding a queue to the YAML needs no code change.
package aws

import (
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/skolldire/go-engine/provider/dynamo"
	"github.com/skolldire/go-engine/provider/s3"
	"github.com/skolldire/go-engine/provider/ses"
	"github.com/skolldire/go-engine/provider/sns"
	"github.com/skolldire/go-engine/provider/sqs"
	"github.com/skolldire/go-engine/provider/ssm"
)

// Options returns the engine options for an AWS-backed service: every AWS
// client the configuration declares, discovered at build time.
//
//	eng, err := engine.New(ctx, aws.Options()...)
func Options() []engine.Option {
	return []engine.Option{engine.WithProviderFunc(Providers)}
}

// Providers returns one provider per AWS client declared in cfg.
func Providers(cfg *engine.Config) []engine.Provider {
	var out []engine.Provider

	for _, name := range engine.InstanceNames(cfg.Section(sqs.ConfigKey)) {
		out = append(out, sqs.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(sns.ConfigKey)) {
		out = append(out, sns.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(ses.ConfigKey)) {
		out = append(out, ses.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(s3.ConfigKey)) {
		out = append(out, s3.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(ssm.ConfigKey)) {
		out = append(out, ssm.New(name))
	}
	for _, name := range engine.InstanceNames(cfg.Section(dynamo.ConfigKey)) {
		out = append(out, dynamo.New(name))
	}

	return out
}
