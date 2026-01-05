package adapters

import (
	"fmt"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// normalizeSQSError normalizes an SQS-related error into a *cloud.Error.
// If err is nil, it returns nil.
// The resulting error uses "<operation>.error" as the code, preserves the original error as the cause, and includes metadata "status_code" = 500.
func normalizeSQSError(err error, operation string) *cloud.Error {
	if err == nil {
		return nil
	}

	// Check for specific SQS error types
	// Note: Type checking for AWS errors would require importing specific error types
	// For now, we'll use generic error handling

	// Generic error normalization
	return cloud.NewErrorWithCause(
		fmt.Sprintf("%s.error", operation),
		err.Error(),
		err,
	).WithMetadata("status_code", 500)
}

// normalizeSNSError converts an AWS SNS error into a *cloud.Error using an
// operation-specific error code ("<operation>.error") and attaches metadata
// "status_code" = 500. It returns nil if the input error is nil.
func normalizeSNSError(err error, operation string) *cloud.Error {
	if err == nil {
		return nil
	}

	return cloud.NewErrorWithCause(
		fmt.Sprintf("%s.error", operation),
		err.Error(),
		err,
	).WithMetadata("status_code", 500)
}

// normalizeLambdaError converts an AWS Lambda error into a *cloud.Error.
// It returns nil if err is nil. The returned error has a code formatted as
// "<operation>.error", uses err.Error() as the message, preserves err as the cause,
// and includes metadata "status_code" set to 500.
func normalizeLambdaError(err error, operation string) *cloud.Error {
	if err == nil {
		return nil
	}

	return cloud.NewErrorWithCause(
		fmt.Sprintf("%s.error", operation),
		err.Error(),
		err,
	).WithMetadata("status_code", 500)
}
