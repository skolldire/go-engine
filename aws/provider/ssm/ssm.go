// Package ssm contributes a SSM client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package ssm

import (
	awsssm "github.com/skolldire/go-engine/aws/pkg/clients/ssm"
	"github.com/skolldire/go-engine/aws/provider/awsbase"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "ssm_clients"

// componentPrefix namespaces this adapter's components, e.g. "ssm:orders".
const componentPrefix = "ssm"

// Provider builds one named SSM client. The behaviour is shared with every
// other AWS adapter, so it lives in awsbase rather than being copied here.
type Provider = awsbase.Provider[awsssm.Config, awsssm.Service]

// New returns a provider for the SSM client declared under instance.
func New(instance string) *Provider {
	return awsbase.NewProvider(componentPrefix, ConfigKey, instance, awsssm.NewClient)
}

// From retrieves the SSM client registered under instance.
func From(e *engine.Engine, instance string) (awsssm.Service, error) {
	return engine.Get[awsssm.Service](e, componentPrefix+":"+instance)
}
