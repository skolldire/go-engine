package helpers

import "fmt"

// WrapError wraps an error with a message, preserving the original error.
// This is a common pattern for adding context to errors.
//
// Example:
//
//	err := someOperation()
//	if err != nil {
//	    return WrapError(err, "failed to process data")
//	}
func WrapError(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// WrapErrorf wraps an error with a formatted message, preserving the original error.
// This allows for formatted error messages with context.
//
// Example:
//
//	err := someOperation()
//	if err != nil {
//	    return WrapErrorf(err, "failed to process user %d", userID)
//	}
func WrapErrorf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", msg, err)
}

// NewError creates a new error with the specified message.
// This is a simple wrapper around fmt.Errorf for consistency.
//
// Example:
//
//	return NewError("invalid input")
func NewError(msg string) error {
	return fmt.Errorf("%s", msg)
}

// NewErrorf creates a new error with a formatted message.
// This is a simple wrapper around fmt.Errorf for consistency.
//
// Example:
//
//	return NewErrorf("invalid user ID: %d", userID)
func NewErrorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
