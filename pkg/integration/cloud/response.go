package cloud

import (
	"encoding/json"
	"fmt"
)

// Response represents a normalized AWS operation response
type Response struct {
	// StatusCode is HTTP-like status code
	// 200: Success
	// 201: Created
	// 204: No Content (successful delete)
	// 400: Bad Request (client error)
	// 404: Not Found
	// 429: Too Many Requests (throttling)
	// 500: Internal Server Error
	// 503: Service Unavailable
	StatusCode int

	// Body contains the response payload as raw bytes
	Body []byte

	// Headers carry response metadata
	// Examples:
	//   - SQS: MessageId, ReceiptHandle
	//   - SNS: MessageId
	//   - Lambda: FunctionError, LogResult
	//   - DynamoDB: ConsumedCapacity, ItemCollectionMetrics
	Headers map[string]string

	// Metadata contains AWS-specific response metadata
	Metadata map[string]interface{}
}

// UnmarshalBody unmarshals Body as JSON into the given value
// This is a helper method, not an implementation of json.Unmarshaler
func (r *Response) UnmarshalBody(v interface{}) error {
	if len(r.Body) == 0 {
		return fmt.Errorf("response body is empty")
	}
	return json.Unmarshal(r.Body, v)
}

// BodyString returns Body as string
func (r *Response) BodyString() string {
	return string(r.Body)
}
