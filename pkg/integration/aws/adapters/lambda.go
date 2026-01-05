package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

type lambdaAdapter struct {
	client  *lambda.Client
	timeout time.Duration
	retries RetryPolicy
}

// newLambdaAdapter creates a cloud.Client that invokes AWS Lambda using the provided AWS configuration, request timeout, and retry policy.
func newLambdaAdapter(cfg aws.Config, timeout time.Duration, retries RetryPolicy) cloud.Client {
	return &lambdaAdapter{
		client:  lambda.NewFromConfig(cfg),
		timeout: timeout,
		retries: retries,
	}
}

func (a *lambdaAdapter) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	switch req.Operation {
	case "lambda.invoke":
		return a.invoke(ctx, req)
	default:
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("unsupported Lambda operation: %s", req.Operation))
	}
}

func (a *lambdaAdapter) invoke(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "function name/path is required")
	}

	input := &lambda.InvokeInput{
		FunctionName: aws.String(req.Path),
		Payload:      req.Body,
	}

	// Parse headers for Lambda-specific attributes
	if req.Headers != nil {
		if invocationType, ok := req.Headers["lambda.invocation_type"]; ok {
			input.InvocationType = types.InvocationType(invocationType)
		} else {
			// Default to RequestResponse
			input.InvocationType = types.InvocationTypeRequestResponse
		}

		if qualifier, ok := req.Headers["lambda.qualifier"]; ok {
			input.Qualifier = aws.String(qualifier)
		}
	} else {
		// Default to RequestResponse
		input.InvocationType = types.InvocationTypeRequestResponse
	}

	result, err := a.client.Invoke(ctx, input)
	if err != nil {
		return nil, normalizeLambdaError(err, "lambda.invoke")
	}

	statusCode := int(result.StatusCode)
	if statusCode == 0 {
		statusCode = 200 // Default to 200 if not set
	}

	headers := make(map[string]string)
	if result.FunctionError != nil {
		headers["lambda.function_error"] = *result.FunctionError
		statusCode = 500 // Function error
	}
	if result.LogResult != nil {
		headers["lambda.log_result"] = *result.LogResult
	}

	return &cloud.Response{
		StatusCode: statusCode,
		Body:       result.Payload,
		Headers:    headers,
		Metadata: map[string]interface{}{
			"lambda.executed_version": result.ExecutedVersion,
		},
	}, nil
}
