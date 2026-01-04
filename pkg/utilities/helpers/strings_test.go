package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal string", "  hello  ", "hello"},
		{"no spaces", "hello", "hello"},
		{"only spaces", "   ", ""},
		{"empty", "", ""},
		{"tabs and newlines", "\t\nhello\t\n", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Trim(tt.input))
		})
	}
}

func TestTrimAndCheckEmpty(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedStr  string
		expectedBool bool
	}{
		{"non-empty", "  hello  ", "hello", false},
		{"empty", "   ", "", true},
		{"empty string", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str, isEmpty := TrimAndCheckEmpty(tt.input)
			assert.Equal(t, tt.expectedStr, str)
			assert.Equal(t, tt.expectedBool, isEmpty)
		})
	}
}

func TestIsWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"whitespace", "   ", true},
		{"empty", "", true},
		{"non-empty", "hello", false},
		{"with content", "  hello  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsWhitespace(tt.input))
		})
	}
}

func TestRemoveChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		chars    []string
		expected string
	}{
		{"remove single char", "hello", []string{"l"}, "heo"},
		{"remove multiple chars", "hello", []string{"l", "o"}, "he"},
		{"no matches", "hello", []string{"x"}, "hello"},
		{"empty chars", "hello", []string{}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RemoveChars(tt.input, tt.chars...))
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normalize", "  Hello World  ", "hello world"},
		{"already normalized", "hello", "hello"},
		{"uppercase", "HELLO", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Normalize(tt.input))
		})
	}
}

func TestCleanSpaces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"multiple spaces", "hello    world", "hello world"},
		{"tabs and spaces", "hello\t\tworld", "hello world"},
		{"leading/trailing", "  hello world  ", "hello world"},
		{"normal", "hello world", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, CleanSpaces(tt.input))
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"no truncation", "hello", 10, "hello"},
		{"truncate with ellipsis", "hello world", 8, "hello..."},
		{"exact length", "hello", 5, "hello"},
		{"very short max", "hello", 2, "he"},
		{"max 3", "hello", 3, "hel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Truncate(tt.input, tt.maxLen))
		})
	}
}

func TestJoinNonEmpty(t *testing.T) {
	tests := []struct {
		name      string
		separator string
		strs      []string
		expected  string
	}{
		{"normal join", ",", []string{"a", "b", "c"}, "a,b,c"},
		{"with empty", ",", []string{"a", "", "b", "  ", "c"}, "a,b,c"},
		{"all empty", ",", []string{"", "  ", ""}, ""},
		{"single", ",", []string{"a"}, "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, JoinNonEmpty(tt.separator, tt.strs...))
		})
	}
}

