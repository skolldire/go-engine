package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

type ssmAdapter struct {
	client  *ssm.Client
	timeout time.Duration
	retries RetryPolicy
}

func newSSMAdapter(cfg aws.Config, timeout time.Duration, retries RetryPolicy) cloud.Client {
	return &ssmAdapter{
		client:  ssm.NewFromConfig(cfg),
		timeout: timeout,
		retries: retries,
	}
}

func (a *ssmAdapter) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	switch req.Operation {
	case "ssm.get_parameter":
		return a.getParameter(ctx, req)
	case "ssm.get_parameters":
		return a.getParameters(ctx, req)
	case "ssm.put_parameter":
		return a.putParameter(ctx, req)
	case "ssm.delete_parameter":
		return a.deleteParameter(ctx, req)
	case "ssm.get_parameters_by_path":
		return a.getParametersByPath(ctx, req)
	case "ssm.get_parameter_history":
		return a.getParameterHistory(ctx, req)
	case "ssm.describe_parameters":
		return a.describeParameters(ctx, req)
	default:
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("unsupported SSM operation: %s", req.Operation))
	}
}

func (a *ssmAdapter) getParameter(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "parameter name is required")
	}

	decrypt := false
	if req.QueryParams != nil {
		if decryptStr, ok := req.QueryParams["WithDecryption"]; ok && decryptStr == "true" {
			decrypt = true
		}
	}

	input := &ssm.GetParameterInput{
		Name:           aws.String(req.Path),
		WithDecryption: aws.Bool(decrypt),
	}

	result, err := a.client.GetParameter(ctx, input)
	if err != nil {
		return nil, normalizeSSMError(err, "ssm.get_parameter")
	}

	param := mapParameter(result.Parameter)
	body, _ := json.Marshal(param)

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
		Headers: map[string]string{
			"ssm.parameter_name": aws.ToString(result.Parameter.Name),
			"ssm.parameter_type":  string(result.Parameter.Type),
		},
		Metadata: map[string]interface{}{
			"ssm.parameter_name": aws.ToString(result.Parameter.Name),
			"ssm.parameter_type": string(result.Parameter.Type),
			"ssm.version":        result.Parameter.Version,
		},
	}, nil
}

func (a *ssmAdapter) getParameters(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// Parse names from body or query params
	var names []string
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &names); err != nil {
			return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("invalid JSON body: %v", err))
		}
	} else if req.QueryParams != nil {
		if namesStr, ok := req.QueryParams["Names"]; ok {
			// Parse comma-separated names
			parts := strings.Split(namesStr, ",")
			names = make([]string, 0, len(parts))
			for _, part := range parts {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					names = append(names, trimmed)
				}
			}
		}
	}

	if len(names) == 0 {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "parameter names are required")
	}

	decrypt := false
	if req.QueryParams != nil {
		if decryptStr, ok := req.QueryParams["WithDecryption"]; ok && decryptStr == "true" {
			decrypt = true
		}
	}

	input := &ssm.GetParametersInput{
		Names:          names,
		WithDecryption: aws.Bool(decrypt),
	}

	result, err := a.client.GetParameters(ctx, input)
	if err != nil {
		return nil, normalizeSSMError(err, "ssm.get_parameters")
	}

	params := make(map[string]interface{})
	for _, param := range result.Parameters {
		params[aws.ToString(param.Name)] = mapParameter(&param)
	}

	body, _ := json.Marshal(params)

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
		Headers: map[string]string{
			"ssm.parameter_count": fmt.Sprintf("%d", len(result.Parameters)),
		},
	}, nil
}

func (a *ssmAdapter) putParameter(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "parameter name is required")
	}

	// Parse body as JSON
	var paramData map[string]interface{}
	if err := json.Unmarshal(req.Body, &paramData); err != nil {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("invalid JSON body: %v", err))
	}

	value, ok := paramData["value"].(string)
	if !ok {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "value is required")
	}

	paramType := types.ParameterTypeString
	if typeStr, ok := paramData["type"].(string); ok && typeStr != "" {
		paramType = types.ParameterType(typeStr)
	}

	overwrite := false
	if overwriteVal, ok := paramData["overwrite"].(bool); ok {
		overwrite = overwriteVal
	}

	input := &ssm.PutParameterInput{
		Name:      aws.String(req.Path),
		Value:     aws.String(value),
		Type:      paramType,
		Overwrite: aws.Bool(overwrite),
	}

	if description, ok := paramData["description"].(string); ok && description != "" {
		input.Description = aws.String(description)
	}

	// Parse tags if present
	if tags, ok := paramData["tags"].(map[string]interface{}); ok {
		tagList := make([]types.Tag, 0, len(tags))
		for k, v := range tags {
			tagList = append(tagList, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(fmt.Sprintf("%v", v)),
			})
		}
		if len(tagList) > 0 {
			input.Tags = tagList
		}
	}

	result, err := a.client.PutParameter(ctx, input)
	if err != nil {
		return nil, normalizeSSMError(err, "ssm.put_parameter")
	}

	return &cloud.Response{
		StatusCode: 200,
		Headers: map[string]string{
			"ssm.version": fmt.Sprintf("%d", result.Version),
		},
		Metadata: map[string]interface{}{
			"ssm.version": result.Version,
		},
	}, nil
}

func (a *ssmAdapter) deleteParameter(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "parameter name is required")
	}

	input := &ssm.DeleteParameterInput{
		Name: aws.String(req.Path),
	}

	_, err := a.client.DeleteParameter(ctx, input)
	if err != nil {
		return nil, normalizeSSMError(err, "ssm.delete_parameter")
	}

	return &cloud.Response{
		StatusCode: 204, // No Content
	}, nil
}

func (a *ssmAdapter) getParametersByPath(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "parameter path is required")
	}

	recursive := false
	if req.QueryParams != nil {
		if recursiveStr, ok := req.QueryParams["Recursive"]; ok && recursiveStr == "true" {
			recursive = true
		}
	}

	decrypt := false
	if req.QueryParams != nil {
		if decryptStr, ok := req.QueryParams["WithDecryption"]; ok && decryptStr == "true" {
			decrypt = true
		}
	}

	var allParams []interface{}
	var nextToken *string

	for {
		input := &ssm.GetParametersByPathInput{
			Path:           aws.String(req.Path),
			Recursive:      aws.Bool(recursive),
			WithDecryption: aws.Bool(decrypt),
			NextToken:      nextToken,
		}

		result, err := a.client.GetParametersByPath(ctx, input)
		if err != nil {
			return nil, normalizeSSMError(err, "ssm.get_parameters_by_path")
		}

		for _, param := range result.Parameters {
			allParams = append(allParams, mapParameter(&param))
		}

		if result.NextToken == nil {
			break
		}
		nextToken = result.NextToken
	}

	body, _ := json.Marshal(allParams)

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
		Headers: map[string]string{
			"ssm.parameter_count": fmt.Sprintf("%d", len(allParams)),
		},
	}, nil
}

func (a *ssmAdapter) getParameterHistory(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "parameter name is required")
	}

	var allHistory []interface{}
	var nextToken *string

	for {
		input := &ssm.GetParameterHistoryInput{
			Name:     aws.String(req.Path),
			NextToken: nextToken,
		}

		result, err := a.client.GetParameterHistory(ctx, input)
		if err != nil {
			return nil, normalizeSSMError(err, "ssm.get_parameter_history")
		}

		for _, hist := range result.Parameters {
			allHistory = append(allHistory, mapParameterHistory(&hist))
		}

		if result.NextToken == nil {
			break
		}
		nextToken = result.NextToken
	}

	body, _ := json.Marshal(allHistory)

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
		Headers: map[string]string{
			"ssm.history_count": fmt.Sprintf("%d", len(allHistory)),
		},
	}, nil
}

func (a *ssmAdapter) describeParameters(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	input := &ssm.DescribeParametersInput{}

	// Parse query params
	if req.QueryParams != nil {
		if path, ok := req.QueryParams["ParameterFilters"]; ok {
			// Simplified - should parse JSON filters
			_ = path
		}
		if maxResults, ok := req.QueryParams["MaxResults"]; ok {
			if max, err := parseInt(maxResults); err == nil {
				input.MaxResults = aws.Int32(int32(max))
			}
		}
	}

	var allParams []interface{}
	var nextToken *string

	for {
		input.NextToken = nextToken

		result, err := a.client.DescribeParameters(ctx, input)
		if err != nil {
			return nil, normalizeSSMError(err, "ssm.describe_parameters")
		}

		for _, param := range result.Parameters {
			allParams = append(allParams, map[string]interface{}{
				"name":             aws.ToString(param.Name),
				"type":             string(param.Type),
				"last_modified_date": param.LastModifiedDate,
				"version":          param.Version,
			})
		}

		if result.NextToken == nil {
			break
		}
		nextToken = result.NextToken
	}

	body, _ := json.Marshal(allParams)

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
		Headers: map[string]string{
			"ssm.parameter_count": fmt.Sprintf("%d", len(allParams)),
		},
	}, nil
}

func normalizeSSMError(err error, operation string) *cloud.Error {
	if err == nil {
		return nil
	}

	// Check for ParameterNotFound using errors.As
	var notFoundErr *types.ParameterNotFound
	if errors.As(err, &notFoundErr) {
		return cloud.NewErrorWithCause(
			cloud.ErrCodeNotFound,
			fmt.Sprintf("Parameter not found: %v", err),
			err,
		).WithMetadata("status_code", 404)
	}

	return normalizeAWSError(err, operation)
}

func mapParameter(param *types.Parameter) map[string]interface{} {
	p := map[string]interface{}{
		"name":  aws.ToString(param.Name),
		"value": aws.ToString(param.Value),
		"type":  string(param.Type),
		"arn":   aws.ToString(param.ARN),
		"version": param.Version,
	}

	if param.LastModifiedDate != nil {
		p["last_modified_date"] = *param.LastModifiedDate
	}

	if param.DataType != nil {
		p["data_type"] = *param.DataType
	}

	return p
}

func mapParameterHistory(hist *types.ParameterHistory) map[string]interface{} {
	ph := map[string]interface{}{
		"name":    aws.ToString(hist.Name),
		"type":    string(hist.Type),
		"value":   aws.ToString(hist.Value),
		"version": hist.Version,
	}

	if hist.LastModifiedDate != nil {
		ph["last_modified_date"] = *hist.LastModifiedDate
	}

	if hist.LastModifiedUser != nil {
		ph["last_modified_user"] = *hist.LastModifiedUser
	}

	if hist.Description != nil {
		ph["description"] = *hist.Description
	}

	if len(hist.Labels) > 0 {
		ph["labels"] = hist.Labels
	}

	return ph
}

