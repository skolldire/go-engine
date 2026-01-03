package adapters

import (
	"fmt"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// normalizeSQSError converts AWS SQS errors to normalized cloud.Error
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

// normalizeSNSError converts AWS SNS errors to normalized cloud.Error
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

// normalizeLambdaError converts AWS Lambda errors to normalized cloud.Error
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

