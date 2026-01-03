package helpers

import "strings"

func IsEmptyString(str string) bool {
	return strings.TrimSpace(str) == ""
}

func IsNotEmptyString(str string) bool {
	return !IsEmptyString(str)
}

func IsEmptySlice[T any](slice []T) bool {
	return len(slice) == 0
}

func IsNotEmptySlice[T any](slice []T) bool {
	return !IsEmptySlice(slice)
}

func IsEmptyMap[K comparable, V any](m map[K]V) bool {
	return len(m) == 0
}

func IsEmptyPtr[T comparable](ptr *T) bool {
	if ptr == nil {
		return true
	}
	var zero T
	return *ptr == zero
}

