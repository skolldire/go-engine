package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsEmptyString(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		expected bool
	}{
		{"empty", "", true},
		{"whitespace", "   ", true},
		{"non-empty", "hello", false},
		{"with content", "  hello  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsEmptyString(tt.str))
		})
	}
}

func TestIsNotEmptyString(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		expected bool
	}{
		{"empty", "", false},
		{"whitespace", "   ", false},
		{"non-empty", "hello", true},
		{"with content", "  hello  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNotEmptyString(tt.str))
		})
	}
}

func TestIsEmptySlice(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		expected bool
	}{
		{"empty", []int{}, true},
		{"nil", nil, true},
		{"non-empty", []int{1, 2}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsEmptySlice(tt.slice))
		})
	}
}

func TestIsNotEmptySlice(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		expected bool
	}{
		{"empty", []int{}, false},
		{"nil", nil, false},
		{"non-empty", []int{1, 2}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNotEmptySlice(tt.slice))
		})
	}
}

func TestIsEmptyMap(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]int
		expected bool
	}{
		{"empty", map[string]int{}, true},
		{"nil", nil, true},
		{"non-empty", map[string]int{"a": 1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsEmptyMap(tt.m))
		})
	}
}

func TestIsEmptyPtr(t *testing.T) {
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
			assert.Equal(t, tt.expected, IsEmptyPtr(tt.ptr))
		})
	}
}
