package observability

import (
	"context"
	"fmt"
	"strings"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/skolldire/go-engine/pkg/utilities/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

// Tracing returns a middleware that traces all requests
func Tracing(tracer telemetry.Tracer) cloud.Middleware {
	return func(next cloud.Client) cloud.Client {
		return &tracingMiddleware{
			next:   next,
			tracer: tracer,
		}
	}
}

type tracingMiddleware struct {
	next   cloud.Client
	tracer telemetry.Tracer
}

func (m *tracingMiddleware) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// If tracer is nil, skip tracing and just call next
	if m.tracer == nil {
		return m.next.Do(ctx, req)
	}

	// Extract service and operation for span name
	service, operation := extractServiceOperation(req.Operation)
	spanName := fmt.Sprintf("%s.%s", service, operation)

	attrs := []attribute.KeyValue{
		attribute.String("aws.service", service),
		attribute.String("aws.operation", operation),
		attribute.String("aws.path", req.Path),
	}

	if req.Method != "" {
		attrs = append(attrs, attribute.String("http.method", req.Method))
	}

	var resp *cloud.Response
	err := m.tracer.Span(ctx, spanName, func(ctx context.Context) error {
		var err error
		resp, err = m.next.Do(ctx, req)

		if err != nil {
			if cloudErr, ok := err.(*cloud.Error); ok {
				attrs = append(attrs,
					attribute.String("aws.error_code", cloudErr.Code),
					attribute.Bool("aws.retriable", cloudErr.Retriable),
				)
			}
			return err
		}

		attrs = append(attrs, attribute.Int("http.status_code", resp.StatusCode))

		// Add AWS request ID if available
		if resp.Metadata != nil {
			if awsReqID, ok := resp.Metadata["aws_request_id"]; ok {
				attrs = append(attrs, attribute.String("aws.request_id", fmt.Sprintf("%v", awsReqID)))
			}
		}

		return nil
	}, attrs...)

	return resp, err
}

// extractServiceOperation extracts service and operation from operation string
func extractServiceOperation(operation string) (service, op string) {
	parts := strings.Split(operation, ".")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return operation, ""
}
