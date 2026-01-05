package cloud

import (
	"fmt"
)

// Error represents a normalized AWS error
type Error struct {
	// Code is a machine-readable error code
	// Format: "service.operation.error_type"
	// Examples:
	//   - "sqs.send.queue_not_found"
	//   - "sns.publish.topic_not_found"
	//   - "lambda.invoke.function_error"
	//   - "dynamodb.put.conditional_check_failed"
	//   - "aws.throttling"
	//   - "aws.authentication_failed"
	Code string

	// Message is human-readable error message
	Message string

	// Retriable indicates if the error is retriable
	Retriable bool

	// Cause is the underlying AWS SDK error (if any)
	Cause error

	// StatusCode is HTTP-like status code
	StatusCode int

	// Metadata contains error-specific metadata
	Metadata map[string]interface{}
}

// Error codes constants
const (
	ErrCodeThrottling            = "aws.throttling"
	ErrCodeAuthenticationFailed  = "aws.authentication_failed"
	ErrCodeAuthorizationFailed   = "aws.authorization_failed"
	ErrCodeServiceUnavailable    = "aws.service_unavailable"
	ErrCodeInvalidRequest         = "aws.invalid_request"
	ErrCodeNotFound               = "aws.not_found"
	ErrCodeConflict               = "aws.conflict"
	ErrCodeConditionalCheckFailed = "aws.conditional_check_failed"
)

// Error implements error interface
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Cause
}

// IsRetriable returns whether the error is retriable
func (e *Error) IsRetriable() bool {
	return e.Retriable
}

// WithMetadata adds metadata to the error
func (e *Error) WithMetadata(key string, value interface{}) *Error {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// NewError creates a new Error
func NewError(code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// NewErrorWithCause creates a new Error with underlying cause
func NewErrorWithCause(code, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}



