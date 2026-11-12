// Package sns contributes a SNS client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package sns

import (
	awssns "github.com/skolldire/go-engine/aws/pkg/clients/sns"
	"github.com/skolldire/go-engine/aws/provider/awsbase"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "sns_clients"

// componentPrefix namespaces this adapter's components, e.g. "sns:orders".
const componentPrefix = "sns"

// Provider builds one named SNS client. The behaviour is shared with every
// other AWS adapter, so it lives in awsbase rather than being copied here.
type Provider = awsbase.Provider[awssns.Config, awssns.Service]

// New returns a provider for the SNS client declared under instance.
func New(instance string) *Provider {
	return awsbase.NewProvider(componentPrefix, ConfigKey, instance, awssns.NewClient)
}

// From retrieves the SNS client registered under instance.
func From(e *engine.Engine, instance string) (awssns.Service, error) {
	return engine.Get[awssns.Service](e, componentPrefix+":"+instance)
}
