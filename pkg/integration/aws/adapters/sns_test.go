package adapters

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
)

func TestSNSAdapter_Do_InvalidOperation(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSNSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sns.invalid_operation",
		Path:      "arn:aws:sns:us-east-1:123:topic",
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SNS operation")
}

func TestSNSAdapter_Publish_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSNSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sns.publish",
		Path:      "", // Empty path
		Body:      []byte("test"),
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "topic ARN/path is required")
}



