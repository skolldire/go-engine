package helpers

// CoalesceString returns the first non-empty string from the provided values.
// This is useful for fallback logic with multiple sources (env vars, config files, defaults).
//
// Example:
//
//	value := CoalesceString(os.Getenv("PORT"), config.Port, "8080")
//	// Returns first non-empty value: env var, config, or default
//
//	value := CoalesceString("", "", "default")
// CoalesceString returns the first non-empty string from the provided values.
// If all values are empty or none are provided, it returns the empty string.
func CoalesceString(values ...string) string {
	for _, v := range values {
		if IsNotEmptyString(v) {
			return v
		}
	}
	return ""
}

// Coalesce returns the first non-zero value from the provided values.
// This is useful for fallback logic with multiple sources of the same type.
//
// Example:
//
//	port := Coalesce(envPort, configPort, 8080)
// Coalesce returns the first value in values that is not the zero value for type T.
// If all provided values are the zero value, the zero value of T is returned.
func Coalesce[T comparable](values ...T) T {
	var zero T
	for _, v := range values {
		if v != zero {
			return v
		}
	}
	return zero
}

// CoalescePtr returns the first non-nil pointer from the provided pointers.
// This is useful for fallback logic with optional pointers.
//
// Example:
//
//	var ptr1 *string
//	ptr2 := Ptr("value")
//	result := CoalescePtr(ptr1, ptr2)
// CoalescePtr returns the first non-nil pointer from the provided pointers.
// If all pointers are nil or no arguments are given, it returns nil.
func CoalescePtr[T any](ptrs ...*T) *T {
	for _, ptr := range ptrs {
		if ptr != nil {
			return ptr
		}
	}
	return nil
}


