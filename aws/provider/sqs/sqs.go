// Package sqs contributes a SQS client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package sqs

import (
	awssqs "github.com/skolldire/go-engine/aws/pkg/clients/sqs"
	"github.com/skolldire/go-engine/aws/provider/awsbase"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "sqs_clients"

// componentPrefix namespaces this adapter's components, e.g. "sqs:orders".
const componentPrefix = "sqs"

// Provider builds one named SQS client. The behaviour is shared with every
// other AWS adapter, so it lives in awsbase rather than being copied here.
type Provider = awsbase.Provider[awssqs.Config, awssqs.Service]

// New returns a provider for the SQS client declared under instance.
func New(instance string) *Provider {
	return awsbase.NewProvider(componentPrefix, ConfigKey, instance, awssqs.NewClient)
}

// From retrieves the SQS client registered under instance.
func From(e *engine.Engine, instance string) (awssqs.Service, error) {
	return engine.Get[awssqs.Service](e, componentPrefix+":"+instance)
}
