package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/skolldire/go-engine/pkg/utilities/telemetry"
)

// MetricsRecorder interface for recording metrics
type MetricsRecorder interface {
	RecordRequest(operation string, duration time.Duration, statusCode int, errorCode string)
	RecordRetry(operation string)
	RecordThrottle(operation string)
}

// Metrics constructs a cloud.Middleware that instruments cloud.Client calls with the given MetricsRecorder.
// The middleware records request duration, status and error codes, and emits retry and throttle events via the provided recorder.
func Metrics(recorder MetricsRecorder) cloud.Middleware {
	return func(next cloud.Client) cloud.Client {
		return &metricsMiddleware{
			next:     next,
			recorder: recorder,
		}
	}
}

type metricsMiddleware struct {
	next     cloud.Client
	recorder MetricsRecorder
}

func (m *metricsMiddleware) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	startTime := time.Now()

	resp, err := m.next.Do(ctx, req)

	duration := time.Since(startTime)

	statusCode := 500
	errorCode := ""
	if err != nil {
		if cloudErr, ok := err.(*cloud.Error); ok {
			statusCode = cloudErr.StatusCode
			errorCode = cloudErr.Code
			if cloudErr.Code == cloud.ErrCodeThrottling {
				m.recorder.RecordThrottle(req.Operation)
			}
		}
		m.recorder.RecordRequest(req.Operation, duration, statusCode, errorCode)
		return nil, err
	}

	statusCode = resp.StatusCode
	m.recorder.RecordRequest(req.Operation, duration, statusCode, "")

	return resp, nil
}

// TelemetryMetricsRecorder implements MetricsRecorder using telemetry.Telemetry
type TelemetryMetricsRecorder struct {
	telemetry telemetry.Telemetry
}

// NewTelemetryMetricsRecorder returns a MetricsRecorder that emits request, retry, and throttle metrics using the provided telemetry.Telemetry.
func NewTelemetryMetricsRecorder(tel telemetry.Telemetry) MetricsRecorder {
	return &TelemetryMetricsRecorder{
		telemetry: tel,
	}
}

func (r *TelemetryMetricsRecorder) RecordRequest(operation string, duration time.Duration, statusCode int, errorCode string) {
	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
		attribute.Int("status_code", statusCode),
	}
	if errorCode != "" {
		attrs = append(attrs, attribute.String("error_code", errorCode))
	}

	// Record duration as histogram
	r.telemetry.Histogram(context.Background(), "aws.request.duration", duration.Seconds(), attrs...)

	// Record count
	r.telemetry.Counter(context.Background(), "aws.request.count", 1, attrs...)

	if statusCode >= 400 {
		r.telemetry.Counter(context.Background(), "aws.request.error", 1, attrs...)
	}
}

func (r *TelemetryMetricsRecorder) RecordRetry(operation string) {
	r.telemetry.Counter(context.Background(), "aws.request.retry", 1,
		attribute.String("operation", operation),
	)
}

func (r *TelemetryMetricsRecorder) RecordThrottle(operation string) {
	r.telemetry.Counter(context.Background(), "aws.request.throttle", 1,
		attribute.String("operation", operation),
	)
}
