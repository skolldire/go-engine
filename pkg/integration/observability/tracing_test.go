package observability

import (
	"context"
	"testing"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTracingMiddleware_Success(t *testing.T) {
	mockTrac := new(mockTracer)
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "my-queue",
	}

	resp := &cloud.Response{
		StatusCode: 200,
		Body:       []byte(`{"messageId":"123"}`),
		Metadata:   map[string]interface{}{"aws_request_id": "req-123"},
	}

	mockCli.On("Do", ctx, req).Return(resp, nil)
	mockTrac.On("Span", ctx, "sqs.send_message", mock.AnythingOfType("func(context.Context) error"),
		mock.Anything).Run(func(args mock.Arguments) {
		// Execute the callback function to populate attributes
		fn := args.Get(2).(func(context.Context) error)
		fn(ctx)
	}).Return(nil)

	middleware := Tracing(mockTrac)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, resp, result)
	mockCli.AssertExpectations(t)
	mockTrac.AssertExpectations(t)
}

func TestTracingMiddleware_Error(t *testing.T) {
	mockTrac := new(mockTracer)
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "my-queue",
	}

	cloudErr := cloud.NewError("sqs.send_message.error", "queue not found")
	cloudErr.Retriable = false

	mockCli.On("Do", ctx, req).Return(nil, cloudErr)
	// The Span method should call the function and return its error
	mockTrac.On("Span", ctx, "sqs.send_message", mock.AnythingOfType("func(context.Context) error"), mock.Anything).Run(func(args mock.Arguments) {
		fn := args.Get(2).(func(context.Context) error)
		fn(ctx) // Call the function which will call mockCli.Do and return cloudErr
	}).Return(cloudErr)

	middleware := Tracing(mockTrac)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, cloudErr, err)
	mockCli.AssertExpectations(t)
	mockTrac.AssertExpectations(t)
}

func TestTracingMiddleware_WithMethod(t *testing.T) {
	mockTrac := new(mockTracer)
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
	mockTrac.On("Span", ctx, "apigateway.proxy", mock.AnythingOfType("func(context.Context) error"), mock.Anything).Return(nil)

	middleware := Tracing(mockTrac)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, resp, result)
	mockCli.AssertExpectations(t)
	mockTrac.AssertExpectations(t)
}

func TestExtractServiceOperation(t *testing.T) {
	tests := []struct {
		name        string
		operation   string
		wantService string
		wantOp      string
	}{
		{
			name:        "valid operation",
			operation:   "sqs.send_message",
			wantService: "sqs",
			wantOp:      "send_message",
		},
		{
			name:        "single part",
			operation:   "sqs",
			wantService: "sqs",
			wantOp:      "",
		},
		{
			name:        "three parts",
			operation:   "aws.sqs.send",
			wantService: "aws",
			wantOp:      "sqs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, op := extractServiceOperation(tt.operation)
			assert.Equal(t, tt.wantService, service)
			assert.Equal(t, tt.wantOp, op)
		})
	}
}

func TestTracingMiddleware_NilTracer(t *testing.T) {
	// When tracer is nil, the middleware should still work but without tracing
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "my-queue",
	}

	resp := &cloud.Response{
		StatusCode: 200,
		Body:       []byte(`{"messageId":"123"}`),
	}

	mockCli.On("Do", ctx, req).Return(resp, nil)

	middleware := Tracing(nil)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, resp, result)
	mockCli.AssertExpectations(t)
}
