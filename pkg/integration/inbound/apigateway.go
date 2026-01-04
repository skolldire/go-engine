package inbound

import (
	"encoding/base64"

	"github.com/aws/aws-lambda-go/events"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// NormalizeAPIGatewayEvent converts API Gateway event to normalized Request
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



