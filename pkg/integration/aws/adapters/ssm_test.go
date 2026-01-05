package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
)

func TestSSMAdapter_Do_InvalidOperation(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSSMAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ssm.invalid_operation",
		Path:      "/my/parameter",
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SSM operation")
}

func TestSSMAdapter_GetParameter_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSSMAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ssm.get_parameter",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parameter name is required")
}

func TestSSMAdapter_GetParameters_InvalidBody(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSSMAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ssm.get_parameters",
		Body:      []byte("invalid json"),
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON body")
}

func TestSSMAdapter_PutParameter_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSSMAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ssm.put_parameter",
		Path:      "", // Empty path
		Body:      []byte("value"),
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parameter name is required")
}

func TestSSMAdapter_DeleteParameter_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSSMAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ssm.delete_parameter",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parameter name is required")
}

func TestSSMAdapter_GetParametersByPath_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSSMAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ssm.get_parameters_by_path",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parameter path is required")
}

func TestSSMAdapter_GetParameterHistory_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSSMAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ssm.get_parameter_history",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parameter name is required")
}

func TestSSMAdapter_DescribeParameters(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSSMAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ssm.describe_parameters",
	}

	// This will fail because we don't have real AWS credentials
	resp, err := adapter.Do(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestNormalizeSSMError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		operation string
		wantNil   bool
		wantCode  string
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			operation: "ssm.get_parameter",
			wantNil:   true,
		},
		{
			name:      "error normalized correctly",
			err:       errors.New("parameter not found"),
			operation: "ssm.get_parameter",
			wantNil:   false,
			wantCode:  "ssm.get_parameter.error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSSMError(tt.err, tt.operation)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantCode, result.Code)
				assert.Equal(t, tt.err.Error(), result.Message)
				assert.NotNil(t, result.Cause)
			}
		})
	}
}

