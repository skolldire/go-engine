package helpers

import (
	"regexp"
	"strings"
)

var multipleSpacesRegex = regexp.MustCompile(`\s+`)

func Trim(str string) string {
	return strings.TrimSpace(str)
}

func TrimAndCheckEmpty(str string) (string, bool) {
	trimmed := strings.TrimSpace(str)
	return trimmed, trimmed == ""
}

func IsWhitespace(str string) bool {
	return strings.TrimSpace(str) == ""
}

func RemoveChars(str string, chars ...string) string {
	result := str
	for _, char := range chars {
		result = strings.ReplaceAll(result, char, "")
	}
	return result
}

func Normalize(str string) string {
	return strings.ToLower(strings.TrimSpace(str))
}

func CleanSpaces(str string) string {
	return multipleSpacesRegex.ReplaceAllString(strings.TrimSpace(str), " ")
}

func Truncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	if maxLen <= 3 {
		return str[:maxLen]
	}
	return str[:maxLen-3] + "..."
}

func JoinNonEmpty(separator string, strs ...string) string {
	var parts []string
	for _, str := range strs {
		if trimmed := strings.TrimSpace(str); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, separator)
}
