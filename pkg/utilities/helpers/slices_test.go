package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		value    int
		expected bool
	}{
		{"contains", []int{1, 2, 3}, 2, true},
		{"does not contain", []int{1, 2, 3}, 4, false},
		{"empty slice", []int{}, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Contains(tt.slice, tt.value))
		})
	}
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name      string
		slice     []int
		predicate func(int) bool
		expected  []int
	}{
		{"filter evens", []int{1, 2, 3, 4, 5}, func(n int) bool { return n%2 == 0 }, []int{2, 4}},
		{"filter odds", []int{1, 2, 3, 4, 5}, func(n int) bool { return n%2 != 0 }, []int{1, 3, 5}},
		{"empty result", []int{1, 2, 3}, func(n int) bool { return n > 10 }, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Filter(tt.slice, tt.predicate))
		})
	}
}

func TestMap(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		mapper   func(int) int
		expected []int
	}{
		{"double", []int{1, 2, 3}, func(n int) int { return n * 2 }, []int{2, 4, 6}},
		{"square", []int{1, 2, 3}, func(n int) int { return n * n }, []int{1, 4, 9}},
		{"empty", []int{}, func(n int) int { return n * 2 }, []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Map(tt.slice, tt.mapper))
		})
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		name      string
		slice     []int
		predicate func(int) bool
		expected  int
		found     bool
	}{
		{"found", []int{1, 2, 3, 4, 5}, func(n int) bool { return n > 3 }, 4, true},
		{"not found", []int{1, 2, 3}, func(n int) bool { return n > 10 }, 0, false},
		{"empty", []int{}, func(n int) bool { return n > 0 }, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := Find(tt.slice, tt.predicate)
			assert.Equal(t, tt.expected, value)
			assert.Equal(t, tt.found, found)
		})
	}
}

func TestConcat(t *testing.T) {
	tests := []struct {
		name     string
		slices   [][]int
		expected []int
	}{
		{"two slices", [][]int{{1, 2}, {3, 4}}, []int{1, 2, 3, 4}},
		{"three slices", [][]int{{1}, {2}, {3}}, []int{1, 2, 3}},
		{"empty slices", [][]int{{}, {1, 2}}, []int{1, 2}},
		{"no slices", [][]int{}, []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Concat(tt.slices...))
		})
	}
}

func TestFirst(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		expected int
		ok       bool
	}{
		{"non-empty", []int{1, 2, 3}, 1, true},
		{"empty", []int{}, 0, false},
		{"single", []int{42}, 42, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := First(tt.slice)
			assert.Equal(t, tt.expected, value)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

func TestLast(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		expected int
		ok       bool
	}{
		{"non-empty", []int{1, 2, 3}, 3, true},
		{"empty", []int{}, 0, false},
		{"single", []int{42}, 42, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := Last(tt.slice)
			assert.Equal(t, tt.expected, value)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

func TestFirstOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		slice        []int
		defaultValue int
		expected     int
	}{
		{"non-empty", []int{1, 2, 3}, 0, 1},
		{"empty", []int{}, 42, 42},
		{"single", []int{10}, 42, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, FirstOrDefault(tt.slice, tt.defaultValue))
		})
	}
}
