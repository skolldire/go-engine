package ssm

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout = 5 * time.Second
)

const (
	ParameterTypeString       = "String"
	ParameterTypeStringList   = "StringList"
	ParameterTypeSecureString = "SecureString"
)

var (
	ErrParameterNotFound = errors.New("parameter not found")
	ErrInvalidInput      = errors.New("invalid input")
	ErrGetParameter      = errors.New("error getting parameter")
	ErrPutParameter      = errors.New("error putting parameter")
	ErrDeleteParameter   = errors.New("error deleting parameter")
)

type Config struct {
	Region         string            `mapstructure:"region" json:"region"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
	Timeout        time.Duration     `mapstructure:"timeout" json:"timeout"`
}

type Parameter struct {
	Name             string
	Value            string
	Type             string
	Version          int64
	LastModifiedDate time.Time
	ARN              string
	DataType         string
	Description      string
}

type ParameterHistory struct {
	Name             string
	Type             string
	Value            string
	Version          int64
	LastModifiedDate time.Time
	LastModifiedUser string
	Description      string
	Labels           []string
}

type DeleteParametersResult struct {
	Deleted []string
	Invalid []string
}

type Service interface {
	// GetParameter retrieves a single parameter by name.
	// If decrypt is true, SecureString parameters will be decrypted.
	GetParameter(ctx context.Context, name string, decrypt bool) (*Parameter, error)

	// GetParameters retrieves multiple parameters by their names.
	// Returns a map keyed by parameter name.
	GetParameters(ctx context.Context, names []string, decrypt bool) (map[string]*Parameter, error)

	// GetParametersByPath retrieves all parameters under a given path.
	// If recursive is true, includes parameters in sub-paths.
	GetParametersByPath(ctx context.Context, path string, recursive bool, decrypt bool) ([]*Parameter, error)

	// PutParameter creates or updates a parameter.
	// If overwrite is false and parameter exists, returns an error.
	PutParameter(ctx context.Context, name, value, parameterType, description string, overwrite bool, tags map[string]string) error

	// PutSecureParameter creates or updates a SecureString parameter (encrypted).
	PutSecureParameter(ctx context.Context, name, value, description string, overwrite bool, tags map[string]string) error

	// DeleteParameter deletes a single parameter.
	DeleteParameter(ctx context.Context, name string) error

	// DeleteParameters deletes multiple parameters.
	// Returns information about which parameters were deleted and which were invalid.
	DeleteParameters(ctx context.Context, names []string) (*DeleteParametersResult, error)

	// GetParameterHistory retrieves the version history of a parameter.
	GetParameterHistory(ctx context.Context, name string) ([]*ParameterHistory, error)

	// AddTagsToResource adds tags to an SSM resource (parameter, document, etc.).
	AddTagsToResource(ctx context.Context, resourceType, resourceID string, tags map[string]string) error

	// ListTagsForResource retrieves tags associated with an SSM resource.
	ListTagsForResource(ctx context.Context, resourceType, resourceID string) (map[string]string, error)

	// ParameterExists checks if a parameter exists.
	ParameterExists(ctx context.Context, name string) (bool, error)

	// EnableLogging enables or disables logging for this client.
	EnableLogging(enable bool)
}

type SSMClient struct {
	*client.BaseClient
	ssmClient *ssm.Client
	region    string
}
