package helpers

import "time"

func DefaultValue[T comparable](value, defaultValue T) T {
	var zero T
	if value == zero {
		return defaultValue
	}
	return value
}

func DefaultString(str, defaultValue string) string {
	if IsEmptyString(str) {
		return defaultValue
	}
	return str
}

func DefaultDuration(d, defaultValue time.Duration) time.Duration {
	if d == 0 {
		return defaultValue
	}
	return d
}
