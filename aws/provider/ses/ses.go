// Package ses contributes a SES client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package ses

import (
	awsses "github.com/skolldire/go-engine/aws/pkg/clients/ses"
	"github.com/skolldire/go-engine/aws/provider/awsbase"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "ses_clients"

// componentPrefix namespaces this adapter's components, e.g. "ses:orders".
const componentPrefix = "ses"

// Provider builds one named SES client. The behaviour is shared with every
// other AWS adapter, so it lives in awsbase rather than being copied here.
type Provider = awsbase.Provider[awsses.Config, awsses.Service]

// New returns a provider for the SES client declared under instance.
func New(instance string) *Provider {
	return awsbase.NewProvider(componentPrefix, ConfigKey, instance, awsses.NewClient)
}

// From retrieves the SES client registered under instance.
func From(e *engine.Engine, instance string) (awsses.Service, error) {
	return engine.Get[awsses.Service](e, componentPrefix+":"+instance)
}
