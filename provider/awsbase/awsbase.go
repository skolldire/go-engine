// Package awsbase resolves the AWS configuration shared by every AWS-backed
// provider.
//
// It exists so the credential chain is resolved at most once per engine and
// only when an AWS provider is actually registered. Resolving it unconditionally
// used to cost every service an IMDS probe at startup and failed outright where
// no AWS credentials exist at all.
package awsbase

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/skolldire/go-engine/pkg/engine"
)

// ConfigKey is the configuration section holding shared AWS settings.
const ConfigKey = "aws"

// resourceKey namespaces the memoized AWS config inside the engine.
const resourceKey = "awsbase.config"

// Settings is the `aws:` section of the configuration file.
type Settings struct {
	Region   string `mapstructure:"region" json:"region"`
	Endpoint string `mapstructure:"endpoint" json:"endpoint"`
}

// Config returns the AWS configuration for this engine, resolving it once and
// sharing it across every AWS provider. The `aws:` section is read from the
// engine configuration, so a provider never has to be told the region.
func Config(ctx context.Context, deps engine.Deps) (aws.Config, error) {
	value, err := deps.Resource(resourceKey, func() (any, error) {
		var settings Settings
		if deps.Section != nil {
			if err := deps.Section(ConfigKey).Decode(&settings); err != nil {
				return nil, fmt.Errorf("decode %q section: %w", ConfigKey, err)
			}
		}

		opts := []func(*awsconfig.LoadOptions) error{}
		if settings.Region != "" {
			opts = append(opts, awsconfig.WithRegion(settings.Region))
		}

		cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("load aws config: %w", err)
		}
		return cfg, nil
	})
	if err != nil {
		return aws.Config{}, err
	}

	cfg, ok := value.(aws.Config)
	if !ok {
		return aws.Config{}, fmt.Errorf("shared aws config has unexpected type %T", value)
	}
	return cfg, nil
}
