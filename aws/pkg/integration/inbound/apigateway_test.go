package inbound

import (
	"encoding/base64"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestNormalizeAPIGatewayEvent(t *testing.T) {
	tests := []struct {
		name    string
		event   *events.APIGatewayProxyRequest
		wantErr bool
	}{
		{
			name: "valid event",
			event: &events.APIGatewayProxyRequest{
				Path:                  "/api/users",
				HTTPMethod:            "POST",
				Body:                  `{"key":"value"}`,
				Headers:               map[string]string{"Content-Type": "application/json"},
				QueryStringParameters: map[string]string{"param": "value"},
				RequestContext: events.APIGatewayProxyRequestContext{
					RequestID: "req-123",
					Stage:     "prod",
				},
				Resource: "/api/{proxy+}",
			},
			wantErr: false,
		},
		{
			name: "base64 encoded body",
			event: &events.APIGatewayProxyRequest{
				Path:            "/api/users",
				HTTPMethod:      "POST",
				Body:            base64.StdEncoding.EncodeToString([]byte("test body")),
				IsBase64Encoded: true,
				RequestContext: events.APIGatewayProxyRequestContext{
					RequestID: "req-123",
					Stage:     "prod",
				},
			},
			wantErr: false,
		},
		{
			name:    "nil event",
			event:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NormalizeAPIGatewayEvent(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeAPIGatewayEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.event != nil && req == nil {
				t.Errorf("NormalizeAPIGatewayEvent() returned nil request")
				return
			}
			if tt.event != nil {
				if req.Operation != "apigateway.proxy" {
					t.Errorf("Operation = %v, want apigateway.proxy", req.Operation)
				}
				if req.Path != tt.event.Path {
					t.Errorf("Path = %v, want %v", req.Path, tt.event.Path)
				}
				if req.Method != tt.event.HTTPMethod {
					t.Errorf("Method = %v, want %v", req.Method, tt.event.HTTPMethod)
				}
			}
		})
	}
}
