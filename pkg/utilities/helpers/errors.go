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
// WrapError wraps err with the provided message, preserving the original error for unwrapping.
// If err is nil, WrapError returns nil.
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
// WrapErrorf wraps the given error with a formatted context message.
// If err is nil, WrapErrorf returns nil. Otherwise it formats the message
// using format and args and returns a new error that combines the formatted
// message with the original error while preserving the original error via wrapping.
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
// NewError creates an error with the provided message.
func NewError(msg string) error {
	return fmt.Errorf("%s", msg)
}

// NewErrorf creates a new error with a formatted message.
// This is a simple wrapper around fmt.Errorf for consistency.
//
// Example:
//
// NewErrorf formats a message using the provided format and arguments and returns it as an error.
// The resulting error's message is the formatted string.
func NewErrorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}


