package observability

import (
	"context"
	"errors"
	"testing"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestLoggingMiddleware_Success(t *testing.T) {
	mockLog := new(mockLogger)
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "my-queue",
	}

	resp := &cloud.Response{
		StatusCode: 200,
		Body:       []byte(`{"messageId":"123"}`),
		Headers:    map[string]string{"sqs.message_id": "123"},
	}

	mockCli.On("Do", ctx, req).Return(resp, nil)
	mockLog.On("Info", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(fields map[string]interface{}) bool {
		return fields["operation"] == "sqs.send_message" &&
			fields["success"] == true &&
			fields["status_code"] == 200
	})).Return()

	middleware := Logging(mockLog)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, resp, result)
	mockCli.AssertExpectations(t)
	mockLog.AssertExpectations(t)
}

func TestLoggingMiddleware_Error(t *testing.T) {
	mockLog := new(mockLogger)
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "my-queue",
	}

	cloudErr := cloud.NewError("sqs.send_message.error", "queue not found")
	cloudErr.StatusCode = 404
	cloudErr.Retriable = false

	mockCli.On("Do", ctx, req).Return(nil, cloudErr)
	mockLog.On("Error", ctx, cloudErr, mock.MatchedBy(func(fields map[string]interface{}) bool {
		return fields["operation"] == "sqs.send_message" &&
			fields["success"] == false &&
			fields["error_code"] == "sqs.send_message.error"
	})).Return()

	middleware := Logging(mockLog)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, cloudErr, err)
	mockCli.AssertExpectations(t)
	mockLog.AssertExpectations(t)
}

func TestLoggingMiddleware_GenericError(t *testing.T) {
	mockLog := new(mockLogger)
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "my-queue",
	}

	genericErr := errors.New("generic error")

	mockCli.On("Do", ctx, req).Return(nil, genericErr)
	mockLog.On("Error", ctx, genericErr, mock.MatchedBy(func(fields map[string]interface{}) bool {
		return fields["operation"] == "sqs.send_message" &&
			fields["success"] == false &&
			fields["status_code"] == 500
	})).Return()

	middleware := Logging(mockLog)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	mockCli.AssertExpectations(t)
	mockLog.AssertExpectations(t)
}

func TestLoggingMiddleware_WithMethod(t *testing.T) {
	mockLog := new(mockLogger)
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "apigateway.proxy",
		Path:      "/api/users",
		Method:    "POST",
	}

	resp := &cloud.Response{
		StatusCode: 200,
		Body:       []byte(`{"id":"123"}`),
	}

	mockCli.On("Do", ctx, req).Return(resp, nil)
	mockLog.On("Info", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(fields map[string]interface{}) bool {
		return fields["method"] == "POST"
	})).Return()

	middleware := Logging(mockLog)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, resp, result)
	mockCli.AssertExpectations(t)
	mockLog.AssertExpectations(t)
}

func TestExtractServiceVerb(t *testing.T) {
	tests := []struct {
		name        string
		operation   string
		wantService string
		wantVerb    string
	}{
		{
			name:        "valid operation",
			operation:   "sqs.send_message",
			wantService: "sqs",
			wantVerb:    "send_message",
		},
		{
			name:        "single part",
			operation:   "sqs",
			wantService: "sqs",
			wantVerb:    "",
		},
		{
			name:        "three parts",
			operation:   "aws.sqs.send",
			wantService: "aws",
			wantVerb:    "sqs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, verb := extractServiceVerb(tt.operation)
			assert.Equal(t, tt.wantService, service)
			assert.Equal(t, tt.wantVerb, verb)
		})
	}
}
