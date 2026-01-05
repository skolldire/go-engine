package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
)

func TestNewBaseAdapter(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := NewBaseAdapter(cfg, 30*time.Second, RetryPolicy{})
	assert.NotNil(t, adapter)
}

func TestBaseAdapter_Do_NilRequest(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := NewBaseAdapter(cfg, 30*time.Second, RetryPolicy{})

	resp, err := adapter.Do(context.Background(), nil)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request cannot be nil")
}

func TestBaseAdapter_Do_EmptyOperation(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := NewBaseAdapter(cfg, 30*time.Second, RetryPolicy{})

	req := &cloud.Request{Operation: ""}
	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operation is required")
}

func TestBaseAdapter_Do_InvalidOperationFormat(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := NewBaseAdapter(cfg, 30*time.Second, RetryPolicy{})

	req := &cloud.Request{Operation: "invalid"}
	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid operation format")
}

func TestBaseAdapter_Do_UnsupportedService(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := NewBaseAdapter(cfg, 30*time.Second, RetryPolicy{})

	req := &cloud.Request{Operation: "unknown.service"}
	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported service")
}

func TestBaseAdapter_Do_ValidService(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := NewBaseAdapter(cfg, 30*time.Second, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		Body:      []byte(`{"test":"data"}`),
	}

	// This will fail because we don't have real AWS credentials, but it tests routing
	resp, err := adapter.Do(context.Background(), req)
	// We expect an error because we don't have real AWS credentials
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestBaseAdapter_Do_WithTimeout(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := NewBaseAdapter(cfg, 30*time.Second, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		Body:      []byte(`{"test":"data"}`),
		Timeout:   5 * time.Second,
	}

	// This will fail because we don't have real AWS credentials
	resp, err := adapter.Do(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}
