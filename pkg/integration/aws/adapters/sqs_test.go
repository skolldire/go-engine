package adapters

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
)

func TestSQSAdapter_Do_InvalidOperation(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSQSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.invalid_operation",
		Path:      "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SQS operation")
}

func TestSQSAdapter_SendMessage_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSQSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "", // Empty path
		Body:      []byte("test"),
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue URL/path is required")
}

func TestSQSAdapter_ReceiveMessage_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSQSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.receive_message",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue URL/path is required")
}

func TestSQSAdapter_DeleteMessage_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSQSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.delete_message",
		Path:      "", // Empty path
		Headers: map[string]string{
			"sqs.receipt_handle": "handle",
		},
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue URL/path is required")
}

func TestSQSAdapter_DeleteMessage_MissingReceiptHandle(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSQSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.delete_message",
		Path:      "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		Headers:   nil, // Missing receipt handle
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "receipt handle is required")
}

func TestSQSAdapter_CreateQueue_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSQSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.create_queue",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue name is required")
}

func TestSQSAdapter_DeleteQueue_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSQSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.delete_queue",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue URL is required")
}

func TestSQSAdapter_GetQueueURL_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSQSAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "sqs.get_queue_url",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue name is required")
}



