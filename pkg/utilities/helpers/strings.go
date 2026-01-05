package helpers

import (
	"regexp"
	"strings"
)

var multipleSpacesRegex = regexp.MustCompile(`\s+`)

// Trim trims leading and trailing whitespace from the input string.
func Trim(str string) string {
	return strings.TrimSpace(str)
}

// TrimAndCheckEmpty trims leading and trailing whitespace from str and reports whether the result is empty.
// It returns the trimmed string and `true` if the trimmed string is empty, `false` otherwise.
func TrimAndCheckEmpty(str string) (string, bool) {
	trimmed := strings.TrimSpace(str)
	return trimmed, trimmed == ""
}

// IsWhitespace reports whether str consists only of Unicode whitespace characters or is empty.
func IsWhitespace(str string) bool {
	return strings.TrimSpace(str) == ""
}

// RemoveChars removes all occurrences of the provided substrings from str.
// If no substrings are provided, it returns str unchanged.
func RemoveChars(str string, chars ...string) string {
	result := str
	for _, char := range chars {
		result = strings.ReplaceAll(result, char, "")
	}
	return result
}

// Normalize returns the input string trimmed of leading and trailing whitespace and converted to lowercase.
func Normalize(str string) string {
	return strings.ToLower(strings.TrimSpace(str))
}

// CleanSpaces trims leading and trailing whitespace and collapses consecutive
// whitespace characters into a single space.
//
// It returns the input with surrounding whitespace removed and each run of one
// or more whitespace characters replaced by a single ASCII space.
func CleanSpaces(str string) string {
	return multipleSpacesRegex.ReplaceAllString(strings.TrimSpace(str), " ")
}

// Truncate shortens str to at most maxLen characters, appending an ellipsis when space allows.
// If str is maxLen characters or shorter, it is returned unchanged. If maxLen <= 3 the first
// maxLen characters are returned without an ellipsis; otherwise the first maxLen-3 characters
// are returned followed by "..." so the result's length is maxLen.
func Truncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	if maxLen <= 3 {
		return str[:maxLen]
	}
	return str[:maxLen-3] + "..."
}

// JoinNonEmpty joins non-empty, trimmed strings using the given separator.
// Each input string is trimmed of leading and trailing whitespace; empty results are omitted before joining.
func JoinNonEmpty(separator string, strs ...string) string {
	var parts []string
	for _, str := range strs {
		if trimmed := strings.TrimSpace(str); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, separator)
}
