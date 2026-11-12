// Package s3 contributes a S3 client to an engine.
//
// It is a leaf of the dependency graph: the core never imports it, so an
// application that does not register this provider does not link its driver.
package s3

import (
	awss3 "github.com/skolldire/go-engine/aws/pkg/clients/s3"
	"github.com/skolldire/go-engine/aws/provider/awsbase"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section this provider consumes.
const ConfigKey = "s3_clients"

// componentPrefix namespaces this adapter's components, e.g. "s3:orders".
const componentPrefix = "s3"

// Provider builds one named S3 client. The behaviour is shared with every
// other AWS adapter, so it lives in awsbase rather than being copied here.
type Provider = awsbase.Provider[awss3.Config, awss3.Service]

// New returns a provider for the S3 client declared under instance.
func New(instance string) *Provider {
	return awsbase.NewProvider(componentPrefix, ConfigKey, instance, awss3.NewClient)
}

// From retrieves the S3 client registered under instance.
func From(e *engine.Engine, instance string) (awss3.Service, error) {
	return engine.Get[awss3.Service](e, componentPrefix+":"+instance)
}
