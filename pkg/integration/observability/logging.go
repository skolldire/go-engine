package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// Logging returns a cloud.Middleware that wraps a cloud.Client to log each request's metadata and outcome.
// The middleware records a unique request ID, operation/service/verb, start time, duration, response status (and AWS request ID when present), and error details using the provided logger Service.
func Logging(log logger.Service) cloud.Middleware {
	return func(next cloud.Client) cloud.Client {
		return &loggingMiddleware{
			next:   next,
			logger: log,
		}
	}
}

type loggingMiddleware struct {
	next   cloud.Client
	logger logger.Service
}

func (m *loggingMiddleware) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	requestID := uuid.New().String()
	startTime := time.Now()

	// Extract service and verb from operation
	service, verb := extractServiceVerb(req.Operation)

	// Build log fields
	logFields := map[string]interface{}{
		"request_id": requestID,
		"operation":  req.Operation,
		"service":    service,
		"verb":       verb,
		"path":       req.Path,
	}

	// Add method only if present (mainly for inbound/APIGateway)
	if req.Method != "" {
		logFields["method"] = req.Method
	}

	// Execute request
	resp, err := m.next.Do(ctx, req)

	duration := time.Since(startTime)
	logFields["duration_ms"] = duration.Milliseconds()
	logFields["start_time"] = startTime.Format(time.RFC3339)

	if err != nil {
		// Error case
		logFields["success"] = false
		if cloudErr, ok := err.(*cloud.Error); ok {
			logFields["error_code"] = cloudErr.Code
			logFields["error_message"] = cloudErr.Message
			logFields["retriable"] = cloudErr.Retriable
			logFields["status_code"] = cloudErr.StatusCode
		} else {
			logFields["error_message"] = err.Error()
			logFields["status_code"] = 500
		}

		m.logger.Error(ctx, err, logFields)
		return nil, err
	}

	// Success case
	logFields["success"] = true
	logFields["status_code"] = resp.StatusCode

	// Add AWS request ID if available
	if resp.Metadata != nil {
		if awsReqID, ok := resp.Metadata["aws_request_id"]; ok {
			logFields["aws_request_id"] = awsReqID
		}
	}

	m.logger.Info(ctx, fmt.Sprintf("AWS operation completed: %s", req.Operation), logFields)

	return resp, nil
}

// extractServiceVerb returns the service and verb parsed from an operation string.
// For example, `sqs.send` yields `sqs` as the service and `send` as the verb; if the
// operation contains no dot, the entire operation is returned as the service and
// the verb is an empty string.
func extractServiceVerb(operation string) (service, verb string) {
	parts := strings.Split(operation, ".")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return operation, ""
}
