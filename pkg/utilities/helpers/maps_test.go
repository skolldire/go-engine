package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetOrDefault(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}

	tests := []struct {
		name         string
		m            map[string]int
		key          string
		defaultValue int
		expected     int
	}{
		{"key exists", m, "a", 0, 1},
		{"key not exists", m, "c", 0, 0},
		{"empty map", map[string]int{}, "a", 42, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetOrDefault(tt.m, tt.key, tt.defaultValue))
		})
	}
}

func TestHasKey(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}

	tests := []struct {
		name     string
		m        map[string]int
		key      string
		expected bool
	}{
		{"key exists", m, "a", true},
		{"key not exists", m, "c", false},
		{"empty map", map[string]int{}, "a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasKey(tt.m, tt.key))
		})
	}
}

func TestKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	keys := Keys(m)
	assert.Len(t, keys, 3)
	assert.Contains(t, keys, "a")
	assert.Contains(t, keys, "b")
	assert.Contains(t, keys, "c")
}

func TestValues(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	values := Values(m)
	assert.Len(t, values, 3)
	assert.Contains(t, values, 1)
	assert.Contains(t, values, 2)
	assert.Contains(t, values, 3)
}

func TestMerge(t *testing.T) {
	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"b": 3, "c": 4}
	result := Merge(m1, m2)

	assert.Equal(t, 1, result["a"])
	assert.Equal(t, 3, result["b"]) // m2 overrides m1
	assert.Equal(t, 4, result["c"])
}

func TestNewStringMap(t *testing.T) {
	m := NewStringMap()
	assert.NotNil(t, m)
	assert.Equal(t, 0, len(m))
	m["key"] = "value"
	assert.Equal(t, "value", m["key"])
}

func TestNewStringInterfaceMap(t *testing.T) {
	m := NewStringInterfaceMap()
	assert.NotNil(t, m)
	assert.Equal(t, 0, len(m))
	m["key"] = "value"
	m["number"] = 42
	assert.Equal(t, "value", m["key"])
	assert.Equal(t, 42, m["number"])
}

func TestNewMap(t *testing.T) {
	m := NewMap[string, int]()
	assert.NotNil(t, m)
	assert.Equal(t, 0, len(m))
	m["key"] = 42
	assert.Equal(t, 42, m["key"])
}



