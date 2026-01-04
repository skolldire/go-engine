package aws

import (
	"context"
	"testing"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/mock"
)

// mockClientHelper is a mock implementation of Client for testing helpers
// Uses the same mockClient from client_test.go but with a simpler interface
type mockClientHelper struct {
	mock.Mock
}

func (m *mockClientHelper) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cloud.Response), args.Error(1)
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
			client: func() Client {
				m := &mockClientHelper{}
				m.On("Do", mock.Anything, mock.MatchedBy(func(req *cloud.Request) bool {
					return req.Operation == "sqs.send_message"
				})).Return(&cloud.Response{
					StatusCode: 200,
					Headers:    map[string]string{"sqs.message_id": "msg-123"},
				}, nil)
				return m
			}(),
			queue:   "my-queue",
			payload: map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name: "client error",
			client: func() Client {
				m := &mockClientHelper{}
				m.On("Do", mock.Anything, mock.Anything).Return(nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "test error"))
				return m
			}(),
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
	client := &mockClientHelper{}
	client.On("Do", mock.Anything, mock.MatchedBy(func(req *cloud.Request) bool {
		return req.Operation == "sns.publish"
	})).Return(&cloud.Response{
		StatusCode: 200,
		Headers:    map[string]string{"sns.message_id": "msg-123"},
	}, nil)

	msgID, err := SNSPublish(context.Background(), client, "arn:aws:sns:us-east-1:123:my-topic", map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("SNSPublish() error = %v", err)
	}
	if msgID != "msg-123" {
		t.Errorf("SNSPublish() messageID = %v, want msg-123", msgID)
	}
}

func TestLambdaInvoke(t *testing.T) {
	client := &mockClientHelper{}
	client.On("Do", mock.Anything, mock.MatchedBy(func(req *cloud.Request) bool {
		return req.Operation == "lambda.invoke"
	})).Return(&cloud.Response{
		StatusCode: 200,
		Body:       []byte(`{"result":"success"}`),
	}, nil)

	resp, err := LambdaInvoke(context.Background(), client, "my-function", map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("LambdaInvoke() error = %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("LambdaInvoke() statusCode = %v, want 200", resp.StatusCode)
	}
}

