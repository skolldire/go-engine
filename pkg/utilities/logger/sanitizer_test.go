package logger

import (
	"testing"
)

func TestSanitizeFields(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "sanitize password field",
			input: map[string]interface{}{
				"username": "testuser",
				"password": "secret123",
			},
			expected: map[string]interface{}{
				"username": "testuser",
				"password": "[REDACTED]",
			},
		},
		{
			name: "sanitize token field",
			input: map[string]interface{}{
				"user_id": "123",
				"token":   "abc123xyz",
			},
			expected: map[string]interface{}{
				"user_id": "123",
				"token":   "[REDACTED]",
			},
		},
		{
			name: "sanitize api_key field",
			input: map[string]interface{}{
				"service": "test",
				"api_key": "key123",
			},
			expected: map[string]interface{}{
				"service": "test",
				"api_key": "[REDACTED]",
			},
		},
		{
			name: "sanitize aws credentials",
			input: map[string]interface{}{
				"aws_access_key_id":     "AKIAIOSFODNN7EXAMPLE",
				"aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			expected: map[string]interface{}{
				"aws_access_key_id":     "[REDACTED]",
				"aws_secret_access_key": "[REDACTED]",
			},
		},
		{
			name: "no sensitive fields",
			input: map[string]interface{}{
				"username": "testuser",
				"user_id":  "12345",
			},
			expected: map[string]interface{}{
				"username": "testuser",
				"user_id":  "12345",
			},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty input",
			input:    map[string]interface{}{},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFields(tt.input)

			if tt.input == nil && result != nil {
				t.Errorf("Expected nil for nil input, got %v", result)
				return
			}

			if tt.input != nil && result == nil {
				t.Errorf("Expected non-nil result for non-nil input")
				return
			}

			if tt.input == nil {
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d fields, got %d", len(tt.expected), len(result))
			}

			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("Field %s: expected %v, got %v", k, v, result[k])
				}
			}
		})
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "password in string",
			input:    "password: secret123",
			expected: "password: [REDACTED]",
		},
		{
			name:     "token in string",
			input:    "token=abc123xyz",
			expected: "token=[REDACTED]",
		},
		{
			name:     "no sensitive data",
			input:    "username: testuser",
			expected: "username: testuser",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeString(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestShouldSanitizeField(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{"password", "password", true},
		{"PASSWORD", "PASSWORD", true},
		{"Password", "Password", true},
		{"token", "token", true},
		{"api_key", "api_key", true},
		{"aws_access_key_id", "aws_access_key_id", true},
		{"username", "username", false},
		{"email", "email", false},
		{"user_id", "user_id", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldSanitizeField(tt.field)
			if result != tt.expected {
				t.Errorf("Field %q: expected %v, got %v", tt.field, tt.expected, result)
			}
		})
	}
}

func TestSanitizeFields_CaseInsensitive(t *testing.T) {
	input := map[string]interface{}{
		"Password": "secret123",
		"TOKEN":    "abc123",
		"Api_Key":  "key123",
	}

	result := SanitizeFields(input)

	if result["Password"] != "[REDACTED]" {
		t.Error("Password field not sanitized (case insensitive)")
	}

	if result["TOKEN"] != "[REDACTED]" {
		t.Error("TOKEN field not sanitized (case insensitive)")
	}

	if result["Api_Key"] != "[REDACTED]" {
		t.Error("Api_Key field not sanitized (case insensitive)")
	}
}
