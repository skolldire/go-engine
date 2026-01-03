package cloud

import (
	"encoding/json"
	"fmt"
	"time"
)

// Request represents a normalized AWS operation request
// This is a pure data structure - no execution context
type Request struct {
	// Operation identifies the AWS service and action
	// Format: "service.operation" (e.g., "sqs.send", "sns.publish", "lambda.invoke")
	Operation string

	// Path maps to AWS resource identifier
	// Examples:
	//   - SQS: queue URL or queue name
	//   - SNS: topic ARN or topic name
	//   - Lambda: function name or ARN
	//   - DynamoDB: table name
	//   - S3: bucket/key path
	//   - SSM: parameter name
	Path string

	// Body contains the request payload as raw bytes
	// Use WithJSONBody() helper for ergonomic JSON serialization
	Body []byte

	// Headers carry metadata and AWS-specific attributes
	// Examples:
	//   - SQS: message attributes, delay seconds, group ID
	//   - SNS: message attributes, subject
	//   - Lambda: invocation type, qualifier
	//   - DynamoDB: condition expressions, return values
	//   - S3: content type, metadata, ACL
	Headers map[string]string

	// QueryParams for filtering/list operations
	// Examples:
	//   - SQS: MaxNumberOfMessages, WaitTimeSeconds
	//   - DynamoDB: filter expressions, projection
	QueryParams map[string]string

	// Timeout is optional per-request timeout
	// If zero, uses client default timeout
	Timeout time.Duration

	// Method is optional HTTP-like method (mainly for inbound/APIGateway normalization)
	// For outbound operations, Operation already defines the action
	// Can be omitted for most outbound operations
	Method string // Optional, mainly for inbound events
}

// WithJSONBody sets Body by JSON-marshaling the given value
func (r *Request) WithJSONBody(v interface{}) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON body: %w", err)
	}
	r.Body = body
	return nil
}

// WithBody sets Body directly from bytes
func (r *Request) WithBody(body []byte) *Request {
	r.Body = body
	return r
}

