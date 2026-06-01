package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMetricsMiddleware_Success(t *testing.T) {
	mockRecorder := new(mockMetricsRecorder)
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
	mockRecorder.On("RecordRequest", "sqs.send_message", mock.AnythingOfType("time.Duration"), 200, "").Return()

	middleware := Metrics(mockRecorder)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, resp, result)
	mockCli.AssertExpectations(t)
	mockRecorder.AssertExpectations(t)
}

func TestMetricsMiddleware_Error(t *testing.T) {
	mockRecorder := new(mockMetricsRecorder)
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "my-queue",
	}

	cloudErr := cloud.NewError("sqs.send_message.error", "queue not found")
	cloudErr.StatusCode = 404

	mockCli.On("Do", ctx, req).Return(nil, cloudErr)
	mockRecorder.On("RecordRequest", "sqs.send_message", mock.AnythingOfType("time.Duration"), 404, "sqs.send_message.error").Return()

	middleware := Metrics(mockRecorder)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	mockCli.AssertExpectations(t)
	mockRecorder.AssertExpectations(t)
}

func TestMetricsMiddleware_Throttling(t *testing.T) {
	mockRecorder := new(mockMetricsRecorder)
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "my-queue",
	}

	cloudErr := cloud.NewError(cloud.ErrCodeThrottling, "throttled")
	cloudErr.StatusCode = 429

	mockCli.On("Do", ctx, req).Return(nil, cloudErr)
	mockRecorder.On("RecordRequest", "sqs.send_message", mock.AnythingOfType("time.Duration"), 429, cloud.ErrCodeThrottling).Return()
	mockRecorder.On("RecordThrottle", "sqs.send_message").Return()

	middleware := Metrics(mockRecorder)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	mockCli.AssertExpectations(t)
	mockRecorder.AssertExpectations(t)
}

func TestMetricsMiddleware_GenericError(t *testing.T) {
	mockRecorder := new(mockMetricsRecorder)
	mockCli := new(mockClient)

	ctx := context.Background()
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      "my-queue",
	}

	genericErr := errors.New("generic error")

	mockCli.On("Do", ctx, req).Return(nil, genericErr)
	mockRecorder.On("RecordRequest", "sqs.send_message", mock.AnythingOfType("time.Duration"), 500, "").Return()

	middleware := Metrics(mockRecorder)
	client := middleware(mockCli)

	result, err := client.Do(ctx, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	mockCli.AssertExpectations(t)
	mockRecorder.AssertExpectations(t)
}

func TestTelemetryMetricsRecorder_RecordRequest_Success(t *testing.T) {
	tel := new(mockTelemetry)
	bg := context.Background()

	tel.On("Histogram", bg, "aws.request.duration", mock.AnythingOfType("float64"), mock.Anything).Return()
	tel.On("Counter", bg, "aws.request.count", int64(1), mock.Anything).Return()

	recorder := NewTelemetryMetricsRecorder(tel)
	recorder.RecordRequest("sqs.send", 10*time.Millisecond, 200, "")

	tel.AssertExpectations(t)
}

func TestTelemetryMetricsRecorder_RecordRequest_Error4xx(t *testing.T) {
	tel := new(mockTelemetry)
	bg := context.Background()

	tel.On("Histogram", bg, "aws.request.duration", mock.AnythingOfType("float64"), mock.Anything).Return()
	tel.On("Counter", bg, "aws.request.count", int64(1), mock.Anything).Return()
	tel.On("Counter", bg, "aws.request.error", int64(1), mock.Anything).Return()

	recorder := NewTelemetryMetricsRecorder(tel)
	recorder.RecordRequest("sqs.send", 5*time.Millisecond, 404, "not-found")

	tel.AssertExpectations(t)
}

func TestTelemetryMetricsRecorder_RecordRetry(t *testing.T) {
	tel := new(mockTelemetry)
	bg := context.Background()

	tel.On("Counter", bg, "aws.request.retry", int64(1), mock.Anything).Return()

	NewTelemetryMetricsRecorder(tel).RecordRetry("sqs.send")

	tel.AssertExpectations(t)
}

func TestTelemetryMetricsRecorder_RecordThrottle(t *testing.T) {
	tel := new(mockTelemetry)
	bg := context.Background()

	tel.On("Counter", bg, "aws.request.throttle", int64(1), mock.Anything).Return()

	NewTelemetryMetricsRecorder(tel).RecordThrottle("sqs.send")

	tel.AssertExpectations(t)
}

func TestNewTelemetryMetricsRecorder_ReturnsNonNil(t *testing.T) {
	recorder := NewTelemetryMetricsRecorder(new(mockTelemetry))
	assert.NotNil(t, recorder)
}
