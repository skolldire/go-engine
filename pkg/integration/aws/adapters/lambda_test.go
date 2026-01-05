package adapters

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
)

func TestLambdaAdapter_Do_InvalidOperation(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newLambdaAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "lambda.invalid_operation",
		Path:      "my-function",
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported Lambda operation")
}

func TestLambdaAdapter_Invoke_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newLambdaAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "lambda.invoke",
		Path:      "", // Empty path
		Body:      []byte("{}"),
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "function name/path is required")
}
