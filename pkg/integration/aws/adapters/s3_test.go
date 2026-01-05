package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
)

func TestS3Adapter_Do_InvalidOperation(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newS3Adapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "s3.invalid_operation",
		Path:      "bucket/key",
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported S3 operation")
}

func TestS3Adapter_PutObject_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newS3Adapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "s3.put_object",
		Path:      "invalid", // Missing key
		Body:      []byte("test"),
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path must be in format")
}

func TestS3Adapter_GetObject_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newS3Adapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "s3.get_object",
		Path:      "invalid", // Missing key
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path must be in format")
}

func TestS3Adapter_DeleteObject_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newS3Adapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "s3.delete_object",
		Path:      "invalid", // Missing key
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path must be in format")
}

func TestS3Adapter_HeadObject_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newS3Adapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "s3.head_object",
		Path:      "invalid", // Missing key
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path must be in format")
}

func TestS3Adapter_ListObjects_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newS3Adapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "s3.list_objects",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bucket name is required")
}

func TestS3Adapter_CopyObject_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newS3Adapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "s3.copy_object",
		Path:      "invalid", // Missing key
		Headers: map[string]string{
			"s3.source_bucket": "source-bucket",
			"s3.source_key":    "source-key",
		},
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path must be in format")
}

func TestS3Adapter_CopyObject_MissingHeaders(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newS3Adapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "s3.copy_object",
		Path:      "dest-bucket/dest-key",
		Headers:   nil, // Missing headers
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source bucket and key are required")
}

func TestParseS3Path(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantBucket string
		wantKey    string
	}{
		{"bucket and key", "bucket/key", "bucket", "key"},
		{"bucket only", "bucket", "bucket", ""},
		{"nested key", "bucket/path/to/key", "bucket", "path/to/key"},
		{"empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key := parseS3Path(tt.path)
			assert.Equal(t, tt.wantBucket, bucket)
			assert.Equal(t, tt.wantKey, key)
		})
	}
}

func TestNormalizeS3Error(t *testing.T) {
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
			operation: "s3.put_object",
			wantNil:   true,
		},
		{
			name:      "error normalized correctly",
			err:       errors.New("bucket not found"),
			operation: "s3.put_object",
			wantNil:   false,
			wantCode:  "s3.put_object.error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeS3Error(tt.err, tt.operation)
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
