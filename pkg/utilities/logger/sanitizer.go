package logger

import (
	"regexp"
	"strings"
)

var (
	// Sensitive patterns that should be sanitized in logs
	sensitivePatterns = []*regexp.Regexp{
		// Passwords, tokens, keys
		regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|key|apikey|api_key|access_key|secret_key)\s*[:=]\s*["']?([^"'\s]+)["']?`),
		// AWS credentials
		regexp.MustCompile(`(?i)(aws_access_key_id|aws_secret_access_key|aws_session_token)\s*[:=]\s*["']?([^"'\s]+)["']?`),
		// Authorization headers
		regexp.MustCompile(`(?i)(authorization|auth)\s*[:=]\s*["']?(bearer|basic)\s+([^"'\s]+)["']?`),
		// Credit cards (basic pattern)
		regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`),
		// Email addresses (can be sensitive in some contexts)
		regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
		// IP addresses (can be sensitive)
		regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
	}

	// Fields that should always be sanitized
	sensitiveFieldNames = map[string]bool{
		"password":        true,
		"passwd":          true,
		"pwd":             true,
		"secret":          true,
		"token":           true,
		"key":             true,
		"apikey":          true,
		"api_key":         true,
		"access_key":      true,
		"secret_key":      true,
		"authorization":   true,
		"auth":            true,
		"credit_card":     true,
		"creditcard":      true,
		"card_number":     true,
		"cvv":             true,
		"cvc":             true,
		"ssn":             true,
		"social_security": true,
		"aws_access_key_id":     true,
		"aws_secret_access_key": true,
		"aws_session_token":     true,
	}
)

const (
	sanitizedValue = "[REDACTED]"
)

// SanitizeFields sanitizes sensitive fields in a map
func SanitizeFields(fields map[string]interface{}) map[string]interface{} {
	if fields == nil {
		return nil
	}

	sanitized := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		keyLower := strings.ToLower(k)
		
		// Check if field name is sensitive
		if sensitiveFieldNames[keyLower] {
			sanitized[k] = sanitizedValue
			continue
		}

		// Check if value is a string that matches sensitive patterns
		if strVal, ok := v.(string); ok {
			sanitized[k] = SanitizeString(strVal)
		} else {
			sanitized[k] = v
		}
	}

	return sanitized
}

// SanitizeString sanitizes sensitive information in a string
func SanitizeString(s string) string {
	if s == "" {
		return s
	}

	result := s
	for _, pattern := range sensitivePatterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			// Try to preserve the key part and redact the value
			parts := strings.SplitN(match, ":", 2)
			if len(parts) == 2 {
				return parts[0] + ": " + sanitizedValue
			}
			parts = strings.SplitN(match, "=", 2)
			if len(parts) == 2 {
				return parts[0] + "=" + sanitizedValue
			}
			return sanitizedValue
		})
	}

	return result
}

// ShouldSanitizeField checks if a field name should be sanitized
func ShouldSanitizeField(fieldName string) bool {
	return sensitiveFieldNames[strings.ToLower(fieldName)]
}

