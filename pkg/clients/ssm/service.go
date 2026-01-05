package ssm

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/validation"
)

// NewClient creates an SSM Service configured with the provided AWS config, package Config, and logger.
// If cfg.Region is set it will be used; cfg.Timeout defaults to DefaultTimeout when zero. Logging and resilience
// settings are applied from cfg.
func NewClient(acf aws.Config, cfg Config, log logger.Service) Service {
	ssmClient := ssm.NewFromConfig(acf, func(o *ssm.Options) {
		if cfg.Region != "" {
			o.Region = cfg.Region
		}
	})

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	baseConfig := client.BaseConfig{
		EnableLogging:  cfg.EnableLogging,
		WithResilience: cfg.WithResilience,
		Resilience:     cfg.Resilience,
		Timeout:        timeout,
	}

	c := &SSMClient{
		BaseClient: client.NewBaseClientWithName(baseConfig, log, "SSM"),
		ssmClient:  ssmClient,
		region:     cfg.Region,
	}

	if c.IsLoggingEnabled() {
		log.Debug(context.Background(), "SSM client initialized",
			map[string]interface{}{
				"region": cfg.Region,
			})
	}

	return c
}

func (c *SSMClient) GetParameter(ctx context.Context, name string, decrypt bool) (*Parameter, error) {
	if name == "" {
		return nil, ErrInvalidInput
	}

	result, err := c.Execute(ctx, "GetParameter", func() (interface{}, error) {
		return c.ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
			Name:           aws.String(name),
			WithDecryption: aws.Bool(decrypt),
		})
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrGetParameter.Error())
	}

	response, err := client.SafeTypeAssert[*ssm.GetParameterOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrGetParameter.Error())
	}
	return mapParameter(response.Parameter), nil
}

func (c *SSMClient) GetParameters(ctx context.Context, names []string, decrypt bool) (map[string]*Parameter, error) {
	if len(names) == 0 {
		return nil, ErrInvalidInput
	}

	result, err := c.Execute(ctx, "GetParameters", func() (interface{}, error) {
		return c.ssmClient.GetParameters(ctx, &ssm.GetParametersInput{
			Names:          names,
			WithDecryption: aws.Bool(decrypt),
		})
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrGetParameter.Error())
	}

	response, err := client.SafeTypeAssert[*ssm.GetParametersOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrGetParameter.Error())
	}
	params := make(map[string]*Parameter)

	for _, param := range response.Parameters {
		p := mapParameter(&param)
		params[p.Name] = p
	}

	return params, nil
}

func (c *SSMClient) GetParametersByPath(ctx context.Context, path string, recursive bool, decrypt bool) ([]*Parameter, error) {
	if path == "" {
		return nil, ErrInvalidInput
	}

	allParams := make([]*Parameter, 0, 100)
	var nextToken *string

	for {
		result, err := c.Execute(ctx, "GetParametersByPath", func() (interface{}, error) {
			return c.ssmClient.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
				Path:           aws.String(path),
				Recursive:      aws.Bool(recursive),
				WithDecryption: aws.Bool(decrypt),
				NextToken:      nextToken,
			})
		})

		if err != nil {
			return nil, c.GetLogger().WrapError(err, ErrGetParameter.Error())
		}

		response, err := client.SafeTypeAssert[*ssm.GetParametersByPathOutput](result)
		if err != nil {
			return nil, c.GetLogger().WrapError(err, ErrGetParameter.Error())
		}
		for _, param := range response.Parameters {
			allParams = append(allParams, mapParameter(&param))
		}

		if response.NextToken == nil {
			break
		}
		nextToken = response.NextToken
	}

	return allParams, nil
}

func (c *SSMClient) PutParameter(ctx context.Context, name, value, parameterType, description string, overwrite bool, tags map[string]string) error {
	if name == "" || value == "" {
		return ErrInvalidInput
	}

	if err := validation.GetGlobalValidator().Var(name, "max=2048"); err != nil {
		return fmt.Errorf("%w: parameter name %v", ErrInvalidInput, err)
	}

	if err := validation.GetGlobalValidator().Var(value, "max=4096"); err != nil {
		return fmt.Errorf("%w: parameter value %v", ErrInvalidInput, err)
	}

	if parameterType == "" {
		parameterType = ParameterTypeString
	}

	input := &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      types.ParameterType(parameterType),
		Overwrite: aws.Bool(overwrite),
	}

	if description != "" {
		input.Description = aws.String(description)
	}

	if len(tags) > 0 {
		var tagList []types.Tag
		for k, v := range tags {
			tagList = append(tagList, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		input.Tags = tagList
	}

	_, err := c.Execute(ctx, "PutParameter", func() (interface{}, error) {
		return c.ssmClient.PutParameter(ctx, input)
	})

	if err != nil {
		return c.GetLogger().WrapError(err, ErrPutParameter.Error())
	}

	return nil
}

func (c *SSMClient) PutSecureParameter(ctx context.Context, name, value, description string, overwrite bool, tags map[string]string) error {
	return c.PutParameter(ctx, name, value, ParameterTypeSecureString, description, overwrite, tags)
}

func (c *SSMClient) DeleteParameter(ctx context.Context, name string) error {
	if name == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "DeleteParameter", func() (interface{}, error) {
		return c.ssmClient.DeleteParameter(ctx, &ssm.DeleteParameterInput{
			Name: aws.String(name),
		})
	})

	if err != nil {
		return c.GetLogger().WrapError(err, ErrDeleteParameter.Error())
	}

	return nil
}

func (c *SSMClient) DeleteParameters(ctx context.Context, names []string) (*DeleteParametersResult, error) {
	if len(names) == 0 {
		return nil, ErrInvalidInput
	}

	result, err := c.Execute(ctx, "DeleteParameters", func() (interface{}, error) {
		return c.ssmClient.DeleteParameters(ctx, &ssm.DeleteParametersInput{
			Names: names,
		})
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrDeleteParameter.Error())
	}

	response, err := client.SafeTypeAssert[*ssm.DeleteParametersOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrDeleteParameter.Error())
	}
	return &DeleteParametersResult{
		Deleted: response.DeletedParameters,
		Invalid: response.InvalidParameters,
	}, nil
}

func (c *SSMClient) GetParameterHistory(ctx context.Context, name string) ([]*ParameterHistory, error) {
	if name == "" {
		return nil, ErrInvalidInput
	}

	var allHistory []*ParameterHistory
	var nextToken *string

	for {
		result, err := c.Execute(ctx, "GetParameterHistory", func() (interface{}, error) {
			return c.ssmClient.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
				Name:     aws.String(name),
				NextToken: nextToken,
			})
		})

		if err != nil {
			return nil, c.GetLogger().WrapError(err, ErrGetParameter.Error())
		}

		response, err := client.SafeTypeAssert[*ssm.GetParameterHistoryOutput](result)
		if err != nil {
			return nil, c.GetLogger().WrapError(err, ErrGetParameter.Error())
		}
		for _, hist := range response.Parameters {
			allHistory = append(allHistory, mapParameterHistory(&hist))
		}

		if response.NextToken == nil {
			break
		}
		nextToken = response.NextToken
	}

	return allHistory, nil
}

func (c *SSMClient) AddTagsToResource(ctx context.Context, resourceType, resourceID string, tags map[string]string) error {
	if resourceType == "" || resourceID == "" || len(tags) == 0 {
		return ErrInvalidInput
	}

	var tagList []types.Tag
	for k, v := range tags {
		tagList = append(tagList, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	_, err := c.Execute(ctx, "AddTagsToResource", func() (interface{}, error) {
		return c.ssmClient.AddTagsToResource(ctx, &ssm.AddTagsToResourceInput{
			ResourceType: types.ResourceTypeForTagging(resourceType),
			ResourceId:   aws.String(resourceID),
			Tags:         tagList,
		})
	})

	if err != nil {
		return c.GetLogger().WrapError(err, "error adding tags to resource")
	}

	return nil
}

func (c *SSMClient) ListTagsForResource(ctx context.Context, resourceType, resourceID string) (map[string]string, error) {
	if resourceType == "" || resourceID == "" {
		return nil, ErrInvalidInput
	}

	result, err := c.Execute(ctx, "ListTagsForResource", func() (interface{}, error) {
		return c.ssmClient.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
			ResourceType: types.ResourceTypeForTagging(resourceType),
			ResourceId:   aws.String(resourceID),
		})
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, "error listing tags for resource")
	}

	response, err := client.SafeTypeAssert[*ssm.ListTagsForResourceOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, "error listing tags for resource")
	}
	tags := make(map[string]string)
	for _, tag := range response.TagList {
		if tag.Key != nil && tag.Value != nil {
			tags[*tag.Key] = *tag.Value
		}
	}

	return tags, nil
}

func (c *SSMClient) ParameterExists(ctx context.Context, name string) (bool, error) {
	_, err := c.GetParameter(ctx, name, false)
	if err != nil {
		var notFoundErr *types.ParameterNotFound
		if errors.As(err, &notFoundErr) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *SSMClient) EnableLogging(enable bool) {
	c.SetLogging(enable)
}

// mapParameter converts an AWS SSM `types.Parameter` into the package `Parameter` type.
// It copies Name, Value, Type, ARN, and Version, and copies LastModifiedDate and DataType when present.
// Returns a pointer to the populated `Parameter`.
func mapParameter(param *types.Parameter) *Parameter {
	p := &Parameter{
		Name:  aws.ToString(param.Name),
		Value: aws.ToString(param.Value),
		Type:  string(param.Type),
		ARN:   aws.ToString(param.ARN),
		Version: param.Version,
	}

	if param.LastModifiedDate != nil {
		p.LastModifiedDate = *param.LastModifiedDate
	}

	if param.DataType != nil {
		p.DataType = *param.DataType
	}

	return p
}

// mapParameterHistory converts an AWS SSM ParameterHistory value into the package's ParameterHistory model.
// 
// The returned ParameterHistory copies Name, Type, Value, and Version, and, when present, populates
// LastModifiedDate, LastModifiedUser, Description, and Labels from the source.
func mapParameterHistory(hist *types.ParameterHistory) *ParameterHistory {
	ph := &ParameterHistory{
		Name:    aws.ToString(hist.Name),
		Type:    string(hist.Type),
		Value:   aws.ToString(hist.Value),
		Version: hist.Version,
	}

	if hist.LastModifiedDate != nil {
		ph.LastModifiedDate = *hist.LastModifiedDate
	}

	if hist.LastModifiedUser != nil {
		ph.LastModifiedUser = *hist.LastModifiedUser
	}

	if hist.Description != nil {
		ph.Description = *hist.Description
	}

	if len(hist.Labels) > 0 {
		ph.Labels = hist.Labels
	}

	return ph
}
