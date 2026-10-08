// Package dynamo contributes a DynamoDB client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package dynamo

import (
	awsdynamo "github.com/skolldire/go-engine/aws/pkg/database/dynamo"
	"github.com/skolldire/go-engine/aws/provider/awsbase"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "dynamo_clients"

// componentPrefix namespaces this adapter's components, e.g. "dynamo:orders".
const componentPrefix = "dynamo"

// Provider builds one named DynamoDB client. The behaviour is shared with every
// other AWS adapter, so it lives in awsbase rather than being copied here.
type Provider = awsbase.Provider[awsdynamo.Config, awsdynamo.Service]

// New returns a provider for the DynamoDB client declared under instance.
func New(instance string) *Provider {
	return awsbase.NewProvider(componentPrefix, ConfigKey, instance, awsdynamo.NewClient)
}

// From retrieves the DynamoDB client registered under instance.
func From(e *engine.Engine, instance string) (awsdynamo.Service, error) {
	return engine.Get[awsdynamo.Service](e, componentPrefix+":"+instance)
}
