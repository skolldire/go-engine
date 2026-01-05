package helpers

// ValueOrZero returns the dereferenced value of ptr or the zero value of T if ptr is nil.
func ValueOrZero[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}

// Ptr returns a pointer to v.
// It is a convenience helper for obtaining a *T from a value.
func Ptr[T any](v T) *T {
	return &v
}

// IsNilOrZero reports whether ptr is nil or points to the zero value of T.
// The type parameter T must be comparable so the value can be compared to its zero value.
func IsNilOrZero[T comparable](ptr *T) bool {
	if ptr == nil {
		return true
	}
	var zero T
	return *ptr == zero
}

// ValueOrError returns the value pointed to by ptr when ptr is non-nil.
// If ptr is nil, it returns the zero value of T and the provided error.
func ValueOrError[T any](ptr *T, err error) (T, error) {
	if ptr == nil {
		var zero T
		return zero, err
	}
	return *ptr, nil
}