package cloud

import (
	"errors"
	"testing"
)

func TestError_Error(t *testing.T) {
	err := NewError("test.code", "test message")
	if err.Error() == "" {
		t.Errorf("Error() should return non-empty string")
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("original error")
	err := NewErrorWithCause("test.code", "test message", cause)
	if err.Unwrap() != cause {
		t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), cause)
	}
}

func TestError_IsRetriable(t *testing.T) {
	tests := []struct {
		name      string
		retriable bool
	}{
		{"retriable", true},
		{"not retriable", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewError("test.code", "test message")
			err.Retriable = tt.retriable
			if err.IsRetriable() != tt.retriable {
				t.Errorf("IsRetriable() = %v, want %v", err.IsRetriable(), tt.retriable)
			}
		})
	}
}

func TestError_WithMetadata(t *testing.T) {
	err := NewError("test.code", "test message")
	result := err.WithMetadata("key", "value")

	if result != err {
		t.Errorf("WithMetadata() should return the same error")
	}
	if err.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %v, want value", err.Metadata["key"])
	}
}

func TestErrorConstants(t *testing.T) {
	constants := []string{
		ErrCodeThrottling,
		ErrCodeAuthenticationFailed,
		ErrCodeAuthorizationFailed,
		ErrCodeServiceUnavailable,
		ErrCodeInvalidRequest,
		ErrCodeNotFound,
		ErrCodeConflict,
		ErrCodeConditionalCheckFailed,
	}

	for _, c := range constants {
		if c == "" {
			t.Errorf("Error constant %s should not be empty", c)
		}
	}
}

