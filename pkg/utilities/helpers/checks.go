package helpers

import "strings"

// IsEmptyString reports whether str, after trimming leading and trailing whitespace, is the empty string.
func IsEmptyString(str string) bool {
	return strings.TrimSpace(str) == ""
}

// IsNotEmptyString reports whether the given string contains any non-whitespace characters.
// It returns `true` if `str` contains at least one non-whitespace character, `false` otherwise.
func IsNotEmptyString(str string) bool {
	return !IsEmptyString(str)
}

// IsEmptySlice reports whether the provided slice has length zero.
// It returns true if the slice has no elements, false otherwise.
func IsEmptySlice[T any](slice []T) bool {
	return len(slice) == 0
}

// IsNotEmptySlice reports whether the provided slice has at least one element.
// It returns true if the slice contains one or more elements, false otherwise.
func IsNotEmptySlice[T any](slice []T) bool {
	return !IsEmptySlice(slice)
}

// IsEmptyMap reports whether the provided map has zero entries.
// It returns true if m has no entries (nil maps are considered empty), false otherwise.
func IsEmptyMap[K comparable, V any](m map[K]V) bool {
	return len(m) == 0
}

// IsEmptyPtr reports whether ptr is nil or points to the zero value of T.
// It returns true when ptr is nil or when the dereferenced value equals T's zero value, and false otherwise.
func IsEmptyPtr[T comparable](ptr *T) bool {
	if ptr == nil {
		return true
	}
	var zero T
	return *ptr == zero
}
