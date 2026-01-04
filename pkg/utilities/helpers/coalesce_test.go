package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoalesceString(t *testing.T) {
	tests := []struct {
		name     string
		values   []string
		expected string
	}{
		{"first non-empty", []string{"", "hello", "world"}, "hello"},
		{"all empty", []string{"", "", ""}, ""},
		{"first is non-empty", []string{"hello", "world"}, "hello"},
		{"single empty", []string{""}, ""},
		{"single non-empty", []string{"hello"}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, CoalesceString(tt.values...))
		})
	}
}

func TestCoalesce(t *testing.T) {
	tests := []struct {
		name     string
		values   []int
		expected int
	}{
		{"first non-zero", []int{0, 1, 2}, 1},
		{"all zero", []int{0, 0, 0}, 0},
		{"first is non-zero", []int{1, 2}, 1},
		{"single zero", []int{0}, 0},
		{"single non-zero", []int{1}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Coalesce(tt.values...))
		})
	}
}

func TestCoalescePtr(t *testing.T) {
	val1 := "first"
	val2 := "second"

	tests := []struct {
		name     string
		ptrs     []*string
		expected *string
	}{
		{"first non-nil", []*string{nil, &val1, &val2}, &val1},
		{"all nil", []*string{nil, nil, nil}, nil},
		{"first is non-nil", []*string{&val1, &val2}, &val1},
		{"single nil", []*string{nil}, nil},
		{"single non-nil", []*string{&val1}, &val1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CoalescePtr(tt.ptrs...)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

