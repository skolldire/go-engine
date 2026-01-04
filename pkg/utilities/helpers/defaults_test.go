package helpers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultValue(t *testing.T) {
	tests := []struct {
		name         string
		value        int
		defaultValue int
		expected     int
	}{
		{"zero value uses default", 0, 42, 42},
		{"non-zero value uses value", 10, 42, 10},
		{"negative value uses value", -1, 42, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DefaultValue(tt.value, tt.defaultValue))
		})
	}
}

func TestDefaultString(t *testing.T) {
	tests := []struct {
		name         string
		str          string
		defaultValue string
		expected     string
	}{
		{"empty uses default", "", "default", "default"},
		{"whitespace uses default", "   ", "default", "default"},
		{"non-empty uses value", "hello", "default", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DefaultString(tt.str, tt.defaultValue))
		})
	}
}

func TestDefaultDuration(t *testing.T) {
	tests := []struct {
		name         string
		d            time.Duration
		defaultValue time.Duration
		expected     time.Duration
	}{
		{"zero uses default", 0, time.Second, time.Second},
		{"non-zero uses value", time.Minute, time.Second, time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DefaultDuration(tt.d, tt.defaultValue))
		})
	}
}

func TestDefaultInt(t *testing.T) {
	tests := []struct {
		name         string
		value        int
		defaultValue int
		expected     int
	}{
		{"zero uses default", 0, 42, 42},
		{"non-zero uses value", 10, 42, 10},
		{"negative uses value", -1, 42, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DefaultInt(tt.value, tt.defaultValue))
		})
	}
}



