package adapters

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/smithy-go"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// normalizeAWSError converts AWS errors to normalized cloud.Error
// It attempts to extract HTTP status codes from AWS error types
func normalizeAWSError(err error, operation string) *cloud.Error {
	if err == nil {
		return nil
	}

	statusCode := 500 // Default to 500

	// Try to extract status code from AWS error types
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		// Check for HTTP status code in error code or message
		if apiErr.ErrorCode() != "" {
			// Map common AWS error codes to HTTP status codes
			switch apiErr.ErrorCode() {
			case "Throttling", "ThrottlingException", "TooManyRequestsException":
				statusCode = 429
			case "AccessDenied", "AccessDeniedException":
				statusCode = 403
			case "NotFound", "NotFoundException", "NoSuchKey", "NoSuchBucket":
				statusCode = 404
			case "BadRequest", "InvalidParameter", "InvalidParameterValue":
				statusCode = 400
			case "ServiceUnavailable", "ServiceUnavailableException":
				statusCode = 503
			}
		}
	}

	// Fallback: try to parse status code from error message
	if statusCode == 500 {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "throttl") {
			statusCode = 429
		} else if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
			statusCode = 404
		} else if strings.Contains(errStr, "400") || strings.Contains(errStr, "bad request") {
			statusCode = 400
		}
	}

	return cloud.NewErrorWithCause(
		fmt.Sprintf("%s.error", operation),
		err.Error(),
		err,
	).WithMetadata("status_code", statusCode)
}

// normalizeSQSError converts AWS SQS errors to normalized cloud.Error
// Deprecated: Use normalizeAWSError instead
func normalizeSQSError(err error, operation string) *cloud.Error {
	return normalizeAWSError(err, operation)
}

// normalizeSNSError converts AWS SNS errors to normalized cloud.Error
// Deprecated: Use normalizeAWSError instead
func normalizeSNSError(err error, operation string) *cloud.Error {
	return normalizeAWSError(err, operation)
}

// normalizeLambdaError converts AWS Lambda errors to normalized cloud.Error
// Deprecated: Use normalizeAWSError instead
func normalizeLambdaError(err error, operation string) *cloud.Error {
	return normalizeAWSError(err, operation)
}
