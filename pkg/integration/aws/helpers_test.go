package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// mockClient is a mock implementation of Client for testing
type mockClient struct {
	doFunc func(ctx context.Context, req *cloud.Request) (*cloud.Response, error)
}

func (m *mockClient) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if m.doFunc != nil {
		return m.doFunc(ctx, req)
	}
	return &cloud.Response{StatusCode: 200}, nil
}

func TestSQSSendMessage(t *testing.T) {
	tests := []struct {
		name    string
		client  Client
		queue   string
		payload interface{}
		wantErr bool
	}{
		{
			name: "success",
			client: &mockClient{
				doFunc: func(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
					if req.Operation != "sqs.send_message" {
						return nil, errors.New("wrong operation")
					}
					return &cloud.Response{
						StatusCode: 200,
						Headers:    map[string]string{"sqs.message_id": "msg-123"},
					}, nil
				},
			},
			queue:   "my-queue",
			payload: map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name: "client error",
			client: &mockClient{
				doFunc: func(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
					return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "test error")
				},
			},
			queue:   "my-queue",
			payload: map[string]string{"key": "value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgID, err := SQSSendMessage(context.Background(), tt.client, tt.queue, tt.payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("SQSSendMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && msgID == "" {
				t.Errorf("SQSSendMessage() messageID is empty")
			}
		})
	}
}

func TestSNSPublish(t *testing.T) {
	client := &mockClient{
		doFunc: func(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
			if req.Operation != "sns.publish" {
				return nil, errors.New("wrong operation")
			}
			return &cloud.Response{
				StatusCode: 200,
				Headers:    map[string]string{"sns.message_id": "msg-123"},
			}, nil
		},
	}

	msgID, err := SNSPublish(context.Background(), client, "arn:aws:sns:us-east-1:123:my-topic", map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("SNSPublish() error = %v", err)
	}
	if msgID != "msg-123" {
		t.Errorf("SNSPublish() messageID = %v, want msg-123", msgID)
	}
}

func TestLambdaInvoke(t *testing.T) {
	client := &mockClient{
		doFunc: func(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
			if req.Operation != "lambda.invoke" {
				return nil, errors.New("wrong operation")
			}
			return &cloud.Response{
				StatusCode: 200,
				Body:       []byte(`{"result":"success"}`),
			}, nil
		},
	}

	resp, err := LambdaInvoke(context.Background(), client, "my-function", map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("LambdaInvoke() error = %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("LambdaInvoke() statusCode = %v, want 200", resp.StatusCode)
	}
}

