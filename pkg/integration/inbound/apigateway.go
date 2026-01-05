package inbound

import (
	"encoding/base64"

	"github.com/aws/aws-lambda-go/events"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// NormalizeAPIGatewayEvent converts an AWS API Gateway proxy event into a normalized cloud.Request.
// It maps Path, HTTP method, headers, and query parameters; decodes a base64-encoded body when present; and appends API Gateway context into headers (`apigateway.request_id`, `apigateway.stage`, `apigateway.resource_path`).
// If event is nil, it returns (nil, nil). If base64 decoding of the body fails, it returns a non-nil error.
func NormalizeAPIGatewayEvent(event *events.APIGatewayProxyRequest) (*cloud.Request, error) {
	if event == nil {
		return nil, nil
	}

	req := &cloud.Request{
		Operation:   "apigateway.proxy",
		Path:        event.Path,
		Method:      event.HTTPMethod, // Required for APIGateway
		Headers:     event.Headers,
		QueryParams: event.QueryStringParameters,
	}

	// Parse body as raw bytes
	if event.Body != "" {
		if event.IsBase64Encoded {
			decoded, err := base64.StdEncoding.DecodeString(event.Body)
			if err != nil {
				return nil, err
			}
			req.Body = decoded
		} else {
			req.Body = []byte(event.Body)
		}
	}

	// Add API Gateway context to headers
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers["apigateway.request_id"] = event.RequestContext.RequestID
	req.Headers["apigateway.stage"] = event.RequestContext.Stage
	req.Headers["apigateway.resource_path"] = event.Resource

	return req, nil
}


