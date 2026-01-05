package helpers

import "time"

// DefaultValue returns value when it is not the zero value for its type, otherwise it returns defaultValue.
// For comparable types T, the function compares value to the zero value of T and selects defaultValue when they are equal.
func DefaultValue[T comparable](value, defaultValue T) T {
	var zero T
	if value == zero {
		return defaultValue
	}
	return value
}

// DefaultString returns defaultValue when str is empty (as determined by IsEmptyString), otherwise it returns str.
func DefaultString(str, defaultValue string) string {
	if IsEmptyString(str) {
		return defaultValue
	}
	return str
}

// DefaultDuration returns defaultValue when d is zero; otherwise it returns d.
func DefaultDuration(d, defaultValue time.Duration) time.Duration {
	if d == 0 {
		return defaultValue
	}
	return d
}

// DefaultInt returns defaultValue when value is 0, otherwise returns value.
func DefaultInt(value, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}