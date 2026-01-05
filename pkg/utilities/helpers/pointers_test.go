package helpers

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValueOrZero(t *testing.T) {
	tests := []struct {
		name     string
		ptr      *int
		expected int
	}{
		{"nil returns zero", nil, 0},
		{"non-nil returns value", Ptr(42), 42},
		{"zero value returns zero", Ptr(0), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ValueOrZero(tt.ptr))
		})
	}
}

func TestPtr(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"creates pointer", 42},
		{"zero value", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Ptr(tt.value)
			assert.NotNil(t, result)
			assert.Equal(t, tt.value, *result)
		})
	}
}

func TestIsNilOrZero(t *testing.T) {
	zero := 0
	nonZero := 42

	tests := []struct {
		name     string
		ptr      *int
		expected bool
	}{
		{"nil", nil, true},
		{"zero value", &zero, true},
		{"non-zero", &nonZero, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNilOrZero(tt.ptr))
		})
	}
}

func TestValueOrError(t *testing.T) {
	testErr := errors.New("test error")
	val := 42

	tests := []struct {
		name         string
		ptr          *int
		err          error
		expectedVal  int
		expectedErr  error
	}{
		{"nil returns error", nil, testErr, 0, testErr},
		{"non-nil returns value", &val, testErr, 42, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultVal, resultErr := ValueOrError(tt.ptr, tt.err)
			assert.Equal(t, tt.expectedVal, resultVal)
			assert.Equal(t, tt.expectedErr, resultErr)
		})
	}
}



